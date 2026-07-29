//go:build windows

package tray

import (
	"syscall"
	"unsafe"
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	procFindWindowW       = user32.NewProc("FindWindowW")
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
)

const (
	wsExAppWindow  = 0x00040000
	wsExToolWindow = 0x00000080
)

// GWL_EXSTYLE как отдельная (не uintptr-константа) переменная, чтобы избежать ошибки
// компиляции "constant -20 overflows uintptr" при знаковом -> беззнаковом преобразовании.
var gwlExStyle int32 = -20

// HideFromTaskbar убирает иконку окна с панели задач Windows, оставляя доступ к приложению
// только через трей. Ищет окно по имени класса (см. windows.Options.WindowClassName в main.go)
// и меняет расширенный стиль окна с WS_EX_APPWINDOW на WS_EX_TOOLWINDOW.
func HideFromTaskbar(windowClassName string) {
	classNamePtr, err := syscall.UTF16PtrFromString(windowClassName)
	if err != nil {
		return
	}

	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(classNamePtr)), 0)
	if hwnd == 0 {
		return
	}

	exStyle, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle))
	newStyle := (exStyle &^ uintptr(wsExAppWindow)) | uintptr(wsExToolWindow)
	procSetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle), newStyle)
}
