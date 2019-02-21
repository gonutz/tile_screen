package win

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/gonutz/w32"
)

type WindowOptions struct {
	X, Y          int
	Width, Height int
	ClassName     string
	Title         string
	Cursor        w32.HCURSOR
	// ClassStyle should include w32.CS_OWNDC for OpenGL
	ClassStyle  uint32
	WindowStyle uint
	Background  w32.HBRUSH
}

type MessageCallback func(window w32.HWND, msg uint32, w, l uintptr) uintptr

func DefaultOptions() WindowOptions {
	return WindowOptions{
		X:           w32.CW_USEDEFAULT,
		Y:           w32.CW_USEDEFAULT,
		Width:       w32.CW_USEDEFAULT,
		Height:      w32.CW_USEDEFAULT,
		ClassName:   "window_class",
		Title:       "",
		Cursor:      w32.LoadCursor(0, w32.MakeIntResource(w32.IDC_ARROW)),
		ClassStyle:  0,
		WindowStyle: w32.WS_OVERLAPPEDWINDOW | w32.WS_VISIBLE,
		Background:  0,
	}
}

// NewWindow creates a window.
func NewWindow(opts WindowOptions, f MessageCallback) (w32.HWND, error) {
	if opts.Width == 0 {
		opts.Width = 640
	}
	if opts.Height == 0 {
		opts.Height = 480
	}
	if opts.ClassName == "" {
		opts.ClassName = "window_class"
	}
	if opts.Cursor == 0 {
		opts.Cursor = w32.LoadCursor(0, w32.MakeIntResource(w32.IDC_ARROW))
	}
	if opts.WindowStyle == 0 {
		opts.WindowStyle = w32.WS_OVERLAPPEDWINDOW
	}
	opts.WindowStyle |= w32.WS_VISIBLE

	class := w32.WNDCLASSEX{
		Background: opts.Background,
		WndProc:    syscall.NewCallback(f),
		Cursor:     opts.Cursor,
		ClassName:  syscall.StringToUTF16Ptr(opts.ClassName),
		Style:      opts.ClassStyle,
	}
	atom := w32.RegisterClassEx(&class)
	if atom == 0 {
		return 0, errors.New("win.NewWindow: RegisterClassEx failed")
	}
	window := w32.CreateWindowEx(
		0,
		syscall.StringToUTF16Ptr(opts.ClassName),
		syscall.StringToUTF16Ptr(opts.Title),
		opts.WindowStyle,
		opts.X, opts.Y, opts.Width, opts.Height,
		0, 0, 0, nil,
	)
	if window == 0 {
		return 0, errors.New("win.NewWindow: CreateWindowEx failed")
	}
	return window, nil
}

// SetIconFromExe sets the icon in the window title bar, in the taskbar and when
// using Alt-Tab to switch between applications.
// The icon is loaded from the running executable file using the given resource
// ID. This means that the icon must be embedded in the executable when building
// by using a resource file for example.
func SetIconFromExe(window w32.HWND, resourceID uint16) {
	iconHandle := w32.LoadImage(
		w32.GetModuleHandle(""),
		w32.MakeIntResource(resourceID),
		w32.IMAGE_ICON,
		0,
		0,
		w32.LR_DEFAULTSIZE|w32.LR_SHARED,
	)
	if iconHandle != 0 {
		w32.SendMessage(window, w32.WM_SETICON, w32.ICON_SMALL, uintptr(iconHandle))
		w32.SendMessage(window, w32.WM_SETICON, w32.ICON_SMALL2, uintptr(iconHandle))
		w32.SendMessage(window, w32.WM_SETICON, w32.ICON_BIG, uintptr(iconHandle))
	}
}

// IsFullscreen returns true if the window has a style different from
// WS_OVERLAPPEDWINDOW. The EnableFullscreen function will change the style to
// borderless so this reports whether that function was called on the window.
// It is not a universally valid test for any window to see if it is fullscreen.
// It is intended for use in conjunction with EnableFullscreen and
// DisableFullscreen.
func IsFullscreen(window w32.HWND) bool {
	style := w32.GetWindowLong(window, w32.GWL_STYLE)
	return style&w32.WS_OVERLAPPEDWINDOW == 0
}

// EnableFullscreen makes the window a borderless window that covers the full
// area of the monitor under the window.
// It returns the previous window placement. Store that value and use it with
// DisableFullscreen to reset the window to what it was before.
func EnableFullscreen(window w32.HWND) (windowed w32.WINDOWPLACEMENT) {
	style := w32.GetWindowLong(window, w32.GWL_STYLE)
	var monitorInfo w32.MONITORINFO
	monitor := w32.MonitorFromWindow(window, w32.MONITOR_DEFAULTTOPRIMARY)
	if w32.GetWindowPlacement(window, &windowed) &&
		w32.GetMonitorInfo(monitor, &monitorInfo) {
		w32.SetWindowLong(
			window,
			w32.GWL_STYLE,
			uint32(style & ^w32.WS_OVERLAPPEDWINDOW),
		)
		w32.SetWindowPos(
			window,
			0,
			int(monitorInfo.RcMonitor.Left),
			int(monitorInfo.RcMonitor.Top),
			int(monitorInfo.RcMonitor.Right-monitorInfo.RcMonitor.Left),
			int(monitorInfo.RcMonitor.Bottom-monitorInfo.RcMonitor.Top),
			w32.SWP_NOOWNERZORDER|w32.SWP_FRAMECHANGED,
		)
	}
	w32.ShowCursor(false)
	return
}

// DisableFullscreen makes the window have a border and the standard icons
// (style WS_OVERLAPPEDWINDOW) and places it at the position given by the window
// placement parameter.
// Use this in conjunction with IsFullscreen and EnableFullscreen to toggle a
// window's fullscreen state.
func DisableFullscreen(window w32.HWND, placement w32.WINDOWPLACEMENT) {
	style := w32.GetWindowLong(window, w32.GWL_STYLE)
	w32.SetWindowLong(
		window,
		w32.GWL_STYLE,
		uint32(style|w32.WS_OVERLAPPEDWINDOW),
	)
	w32.SetWindowPlacement(window, &placement)
	w32.SetWindowPos(window, 0, 0, 0, 0, 0,
		w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOZORDER|
			w32.SWP_NOOWNERZORDER|w32.SWP_FRAMECHANGED,
	)
	w32.ShowCursor(true)
}

// RunMainLoop starts the applications window message handling. It loops until
// the window is closed. Messages are forwarded to the handler function that was
// passed to NewWindow.
func RunMainLoop() {
	var msg w32.MSG
	for w32.GetMessage(&msg, 0, 0, 0) != 0 {
		w32.TranslateMessage(&msg)
		w32.DispatchMessage(&msg)
	}
}

// RunMainGameLoop starts the application's window message handling. It loops
// until the window is closed. Messages are forwarded to the handler function
// that was passed to NewWindow.
// In contrast to RunMainLoop, RunMainGameLoop calls the given function whenever
// there are now messages to be handled at the moment. You can use this like a
// classical DOS era endless loop to run any real-time logic in between
// messages.
// Tip: if you do not want the game to use all your CPU, do some kind of
// blocking operation in the function you pass. A simple time.Sleep(0) will do
// the trick.
func RunMainGameLoop(f func()) {
	var msg w32.MSG
	w32.PeekMessage(&msg, 0, 0, 0, w32.PM_NOREMOVE)
	for msg.Message != w32.WM_QUIT {
		if w32.PeekMessage(&msg, 0, 0, 0, w32.PM_REMOVE) {
			w32.TranslateMessage(&msg)
			w32.DispatchMessage(&msg)
		} else {
			f()
		}
	}
}

// CloseWindow sends a WM_CLOSE event to the given window.
func CloseWindow(window w32.HWND) {
	w32.SendMessage(window, w32.WM_CLOSE, 0, 0)
}

// HideConsoleWindow hides the associated console window if it was created
// because the ldflag H=windowsgui was not provided when building.
func HideConsoleWindow() {
	console := w32.GetConsoleWindow()
	if console == 0 {
		return // no console attached
	}
	// If this application is the process that created the console window, then
	// this program was not compiled with the -H=windowsgui flag and on start-up
	// it created a console along with the main application window. In this case
	// hide the console window.
	// See
	// http://stackoverflow.com/questions/9009333/how-to-check-if-the-program-is-run-from-a-console
	// and thanks to
	// https://github.com/hajimehoshi
	// for the tip.
	_, consoleProcID := w32.GetWindowThreadProcessId(console)
	if w32.GetCurrentProcessId() == consoleProcID {
		w32.ShowWindowAsync(console, w32.SW_HIDE)
	}
}

// HandlePanics is designed to be deferred as the first statement in an
// application's main function. It calls recover to catch unhandled panics. The
// current stack is output to standard output, to a file in the user's APPDATA
// folder (which is then opened with the default .txt editor) and to a message
// box that is shown to the user.
// The id is used in the log file name.
func HandlePanics(id string) {
	if err := recover(); err != nil {
		// in case of a panic, create a message with the current stack
		msg := fmt.Sprintf("panic: %v\nstack:\n\n%s\n", err, debug.Stack())

		// print it to stdout
		fmt.Println(msg)

		// write it to a log file
		filename := filepath.Join(
			os.Getenv("APPDATA"),
			id+"_panic_log_"+time.Now().Format("2006_01_02__15_04_05")+".txt",
		)
		ioutil.WriteFile(filename, []byte(msg), 0777)

		// open the log file with the default text viewer
		exec.Command("cmd", "/C", filename).Start()

		// pop up a message box
		w32.MessageBox(
			0,
			msg,
			"The program crashed",
			w32.MB_OK|w32.MB_ICONERROR|w32.MB_TOPMOST,
		)
	}
}

// Callback can be used as the callback function for a window. It will translate
// common messages into nice function calls. No need to handle generic W and L
// parameters yourself.
func (m *MessageHandler) Callback(window w32.HWND, msg uint32, w, l uintptr) uintptr {
	if msg == w32.WM_TIMER && m.OnTimer != nil {
		m.OnTimer(w)
		return 0
	} else if msg == w32.WM_KEYDOWN && m.OnKeyDown != nil {
		m.OnKeyDown(w, KeyOptions(l))
		return 0
	} else if msg == w32.WM_KEYUP && m.OnKeyUp != nil {
		m.OnKeyDown(w, KeyOptions(l))
		return 0
	} else if msg == w32.WM_CHAR && m.OnChar != nil {
		r := utf16.Decode([]uint16{uint16(w)})[0]
		m.OnChar(r)
		return 0
	} else if msg == w32.WM_MOUSEMOVE && m.OnMouseMove != nil {
		x := int((uint(l)) & 0xFFFF)
		y := int((uint(l) >> 16) & 0xFFFF)
		m.OnMouseMove(x, y, MouseOptions(w))
		return 0
	} else if msg == w32.WM_SIZE && m.OnSize != nil {
		w := int((uint(l)) & 0xFFFF)
		h := int((uint(l) >> 16) & 0xFFFF)
		m.OnSize(w, h)
		return 0
	} else if msg == w32.WM_MOVE && m.OnMove != nil {
		x := int((uint(l)) & 0xFFFF)
		y := int((uint(l) >> 16) & 0xFFFF)
		m.OnMove(x, y)
		return 0
	} else if msg == w32.WM_ACTIVATE && m.OnActivate != nil {
		if w != 0 && m.OnActivate != nil {
			m.OnActivate()
		}
		if w == 0 && m.OnDeactivate != nil {
			m.OnDeactivate()
		}
		return 0
	} else if msg == w32.WM_LBUTTONDOWN && m.OnLeftMouseDown != nil {
		m.OnLeftMouseDown(mouseX(l), mouseY(l), MouseOptions(w))
		return 0
	} else if msg == w32.WM_RBUTTONDOWN && m.OnRightMouseDown != nil {
		m.OnRightMouseDown(mouseX(l), mouseY(l), MouseOptions(w))
		return 0
	} else if msg == w32.WM_MBUTTONDOWN && m.OnMiddleMouseDown != nil {
		m.OnMiddleMouseDown(mouseX(l), mouseY(l), MouseOptions(w))
		return 0
	} else if msg == w32.WM_LBUTTONUP && m.OnLeftMouseUp != nil {
		m.OnLeftMouseUp(mouseX(l), mouseY(l), MouseOptions(w))
		return 0
	} else if msg == w32.WM_RBUTTONUP && m.OnRightMouseUp != nil {
		m.OnRightMouseUp(mouseX(l), mouseY(l), MouseOptions(w))
		return 0
	} else if msg == w32.WM_MBUTTONUP && m.OnMiddleMouseUp != nil {
		m.OnMiddleMouseUp(mouseX(l), mouseY(l), MouseOptions(w))
		return 0
	} else if msg == w32.WM_MOUSEWHEEL && m.OnMouseWheel != nil {
		delta := float32(int16((w>>16)&0xFFFF)) / 120.0
		m.OnMouseWheel(delta, mouseX(l), mouseY(l), MouseOptions(w&0xFFFF))
		return 0
	} else if msg == w32.WM_DESTROY {
		w32.PostQuitMessage(0)
		return 0
	} else {
		return w32.DefWindowProc(window, msg, w, l)
	}
}

func mouseX(l uintptr) int {
	return int(int16(l & 0xFFFF))
}

func mouseY(l uintptr) int {
	return int(int16((l >> 16) & 0xFFFF))
}

// MessageHandler translates common Windows messages for you instead of
// providing generic W and L parameters. Set the handlers that you want and
// leave the rest at nil. Use the MessageHandler's Callback function as the
// callback for a window.
type MessageHandler struct {
	OnKeyDown         func(key uintptr, options KeyOptions)
	OnKeyUp           func(key uintptr, options KeyOptions)
	OnMouseMove       func(x, y int, options MouseOptions)
	OnMouseWheel      func(forward float32, x, y int, options MouseOptions)
	OnLeftMouseDown   func(x, y int, options MouseOptions)
	OnRightMouseDown  func(x, y int, options MouseOptions)
	OnMiddleMouseDown func(x, y int, options MouseOptions)
	OnLeftMouseUp     func(x, y int, options MouseOptions)
	OnRightMouseUp    func(x, y int, options MouseOptions)
	OnMiddleMouseUp   func(x, y int, options MouseOptions)
	OnChar            func(r rune)
	OnSize            func(width, height int)
	OnMove            func(x, y int)
	OnActivate        func()
	OnDeactivate      func()
	OnTimer           func(id uintptr)
}

type KeyOptions uintptr

func (o KeyOptions) RepeatCount() int {
	return int(o & 0xFFFF)
}

func (o KeyOptions) ScanCode() int {
	return int((o >> 16) & 0xFF)
}

func (o KeyOptions) IsExtended() bool {
	return o&(1<<24) != 0
}

func (o KeyOptions) WasDown() bool {
	return o&(1<<30) != 0
}

type MouseOptions uintptr

func (o MouseOptions) ControlDown() bool {
	return o&w32.MK_CONTROL != 0
}

func (o MouseOptions) LButtonDown() bool {
	return o&w32.MK_LBUTTON != 0
}

func (o MouseOptions) MButtonDown() bool {
	return o&w32.MK_MBUTTON != 0
}

func (o MouseOptions) RButtonDown() bool {
	return o&w32.MK_RBUTTON != 0
}

func (o MouseOptions) ShiftDown() bool {
	return o&w32.MK_SHIFT != 0
}

func (o MouseOptions) XButton1Down() bool {
	return o&w32.MK_XBUTTON1 != 0
}

func (o MouseOptions) XButton2Down() bool {
	return o&w32.MK_XBUTTON2 != 0
}

func ClientSize(window w32.HWND) (w, h int) {
	r := w32.GetClientRect(window)
	if r == nil {
		return 0, 0
	}
	return int(r.Width()), int(r.Height())
}
