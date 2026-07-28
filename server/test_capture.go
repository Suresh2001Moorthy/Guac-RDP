package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	findWindow = user32.NewProc("FindWindowW")
	showWindow = user32.NewProc("ShowWindow")
)

func main() {
	// Find window by class name "SDL_app" or title.
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("FreeRDP: 192.168.1.241"))))
	if hwnd == 0 {
		fmt.Println("Window not found")
		return
	}
	fmt.Printf("Found HWND: %v\n", hwnd)
	// Try to hide it
	showWindow.Call(hwnd, 0) // SW_HIDE = 0
	fmt.Println("Window hidden")
}
