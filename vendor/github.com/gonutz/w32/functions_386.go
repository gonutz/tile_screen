package w32

// MenuItemFromPoint determines which menu item, if any, is at the specified
// location.
func MenuItemFromPoint(w HWND, m HMENU, screen POINT) int {
	ret, _, _ := menuItemFromPoint.Call(
		uintptr(w),
		uintptr(m),
		uintptr(screen.X),
		uintptr(screen.Y),
	)
	return int(ret)
}
