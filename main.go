package main

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/gonutz/w32"
	"github.com/gonutz/win"
)

func main() {
	runtime.LockOSThread()

	var info w32.MONITORINFO
	var selecting bool
	var selection w32.RECT
	tiles := 2
	window, err := newWindow(
		0, 0, 1, 1,
		"tile_screen_window",
		w32.WS_POPUPWINDOW|w32.WS_VISIBLE,
		func(window w32.HWND, msg uint32, w, l uintptr) uintptr {
			switch msg {
			case w32.WM_MOUSEMOVE:
				if selecting {
					x := int32(int16(w32.LOWORD(uint32(l))))
					y := int32(int16(w32.HIWORD(uint32(l))))
					old := selection
					selection.Left = min(selection.Left, x)
					selection.Top = min(selection.Top, y)
					selection.Right = max(selection.Right, x)
					selection.Bottom = max(selection.Bottom, y)
					if selection != old {
						w32.InvalidateRect(window, nil, false)
					}
				}
				return 0
			case w32.WM_LBUTTONDOWN:
				x := int32(int16(w32.LOWORD(uint32(l))))
				y := int32(int16(w32.HIWORD(uint32(l))))
				selecting = true
				selection = w32.RECT{
					Left:   x,
					Top:    y,
					Right:  x,
					Bottom: y,
				}
				return 0
			case w32.WM_LBUTTONUP:
				if selecting {
					w32.ShowWindow(window, w32.SW_MINIMIZE)
					w := window
					const tickDelay = 100 * time.Millisecond
					for w == window {
						time.Sleep(tickDelay)
						w = w32.GetForegroundWindow()
					}
					if w == 0 || w == w32.GetDesktopWindow() {
						win.CloseWindow(window)
						return 0
					}
					m := w32.MonitorFromWindow(w, w32.MONITOR_DEFAULTTONULL)
					if m == 0 {
						win.CloseWindow(window)
						return 0
					}
					w32.ShowWindow(w, w32.SW_RESTORE)
					tileW := int(info.RcWork.Width()) / tiles
					tileH := int(info.RcWork.Height()) / tiles
					x := int(selection.Left) / tileW * tileW
					y := int(selection.Top) / tileH * tileH
					right := int(selection.Right)/tileW*tileW + tileW
					if right > int(info.RcWork.Width()) {
						right = int(info.RcWork.Width())
					}
					bottom := int(selection.Bottom)/tileH*tileH + tileH
					if bottom > int(info.RcWork.Height()) {
						bottom = int(info.RcWork.Height())
					}
					if int(selection.Right)/tileW == tiles-1 {
						right += int(info.RcWork.Width()) % tileW
					}
					if int(selection.Bottom)/tileH == tiles-1 {
						bottom += int(info.RcWork.Height()) % tileH
					}
					w32.SetWindowPos(
						w, 0,
						int(info.RcWork.Left)+x, int(info.RcWork.Top)+y,
						right-x, bottom-y,
						w32.SWP_ASYNCWINDOWPOS|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER|w32.SWP_NOZORDER|w32.SWP_SHOWWINDOW,
					)

					ioutil.WriteFile(settingsPath(), []byte{byte(tiles)}, 0666)
					win.CloseWindow(window)
				}
				return 0
			case w32.WM_PAINT:
				const (
					backColor = w32.COLOR_HIGHLIGHT
					foreColor = w32.COLOR_BTNFACE
					inColor   = w32.COLOR_DESKTOP
				)
				var ps w32.PAINTSTRUCT
				hdc := w32.BeginPaint(window, &ps)
				w32.FillRect(hdc, &w32.RECT{
					Left:   0,
					Top:    0,
					Right:  info.RcWork.Width(),
					Bottom: info.RcWork.Height(),
				}, backColor)
				w := int(info.RcWork.Width()) / tiles
				h := int(info.RcWork.Height()) / tiles
				for x := 0; x < tiles; x++ {
					for y := 0; y < tiles; y++ {
						r := w32.RECT{
							Left:   int32(x*w) + 2,
							Top:    int32(y*h) + 2,
							Right:  int32((x+1)*w) - 4,
							Bottom: int32((y+1)*h) - 4,
						}
						color := foreColor
						if overlap(r, selection) {
							color = inColor
						}
						w32.FillRect(hdc, &r, w32.HBRUSH(color))
					}
				}
				w32.EndPaint(window, &ps)
				return 0
			case w32.WM_KEYDOWN:
				if !selecting && '2' <= w && w <= '9' {
					tiles = int(w - '0')
					w32.InvalidateRect(window, nil, false)
				} else if w == w32.VK_ESCAPE {
					win.CloseWindow(window)
				}
				return 0
			case w32.WM_DESTROY:
				w32.PostQuitMessage(0)
				return 0
			default:
				return w32.DefWindowProc(window, msg, w, l)
			}

		},
	)
	if err != nil {
		panic(err)
	}

	data, err := ioutil.ReadFile(settingsPath())
	if err == nil {
		tiles = int(min(9, max(2, int32(data[0]))))
	}

	w32.ShowWindow(window, w32.SW_MINIMIZE)
	const tickDelay = 100 * time.Millisecond
	w := window
	for w == window {
		time.Sleep(tickDelay)
		w = w32.GetForegroundWindow()
	}
	monitor := w32.MonitorFromWindow(w, w32.MONITOR_DEFAULTTONULL)
	w32.ShowWindow(window, w32.SW_RESTORE)

	if monitor == 0 {
		panic("no monitor under window detected")
	}
	if !w32.GetMonitorInfo(monitor, &info) {
		panic("unable to query monitor info")
	}
	w32.SetWindowPos(
		window, 0,
		int(info.RcWork.Left), int(info.RcWork.Top),
		int(info.RcWork.Width()), int(info.RcWork.Height()),
		w32.SWP_ASYNCWINDOWPOS|w32.SWP_NOACTIVATE|w32.SWP_NOOWNERZORDER|w32.SWP_NOZORDER|w32.SWP_SHOWWINDOW,
	)

	win.RunMainLoop()
}

type MessageCallback func(window w32.HWND, msg uint32, w, l uintptr) uintptr

func newWindow(x, y, width, height int, className string, style uint, f MessageCallback) (w32.HWND, error) {
	class := w32.WNDCLASSEX{
		WndProc:    syscall.NewCallback(f),
		Cursor:     w32.LoadCursor(0, w32.MakeIntResource(w32.IDC_ARROW)),
		ClassName:  syscall.StringToUTF16Ptr(className),
		Background: w32.COLOR_DESKTOP,
	}
	atom := w32.RegisterClassEx(&class)
	if atom == 0 {
		return 0, errors.New("RegisterClassEx failed")
	}
	window := w32.CreateWindowEx(
		0,
		syscall.StringToUTF16Ptr(className),
		nil,
		style,
		x, y, width, height,
		0, 0, 0, nil,
	)
	if window == 0 {
		return 0, errors.New("CreateWindowEx failed")
	}
	return window, nil
}

func min(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func overlap(a, b w32.RECT) bool {
	return b.Right >= a.Left && b.Left < a.Right &&
		b.Bottom >= a.Top && b.Top < a.Bottom
}

func settingsPath() string {
	return filepath.Join(os.Getenv("APPDATA"), "screen_tile.set")
}
