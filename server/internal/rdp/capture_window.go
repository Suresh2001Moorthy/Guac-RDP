package rdp

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	findWindow         = user32.NewProc("FindWindowW")
	showWindow         = user32.NewProc("ShowWindow")
	getWindowDC        = user32.NewProc("GetWindowDC")
	releaseDC          = user32.NewProc("ReleaseDC")
	printWindow        = user32.NewProc("PrintWindow")
	getClientRect      = user32.NewProc("GetClientRect")
	enumWindows        = user32.NewProc("EnumWindows")
	getWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	isWindowVisible          = user32.NewProc("IsWindowVisible")
	setWindowPos             = user32.NewProc("SetWindowPos")
	setWindowLong            = user32.NewProc("SetWindowLongW")
	getWindowLong            = user32.NewProc("GetWindowLongW")
	setLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")

	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	createCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteObject       = gdi32.NewProc("DeleteObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
	getDIBits          = gdi32.NewProc("GetDIBits")
)

type rect struct {
	Left, Top, Right, Bottom int32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors *rgbQuad
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type rgbQuad struct {
	RgbBlue     byte
	RgbGreen    byte
	RgbRed      byte
	RgbReserved byte
}

func FindWindowByPID(pid int) uintptr {
	var targetHwnd uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, lParam uintptr) uintptr {
		var processId uint32
		getWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&processId)))
		if int(processId) == pid {
			res, _, _ := isWindowVisible.Call(hwnd)
			if res != 0 {
				targetHwnd = hwnd
				return 0 // stop enumerating
			}
		}
		return 1 // continue enumerating
	})
	enumWindows.Call(cb, 0)
	return targetHwnd
}

func FindRDPWindow(title string) uintptr {
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))))
	return hwnd
}

const (
	GWL_EXSTYLE       = -20
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TOOLWINDOW  = 0x00000080
	LWA_ALPHA         = 0x00000002
)

func HideWindow(hwnd uintptr) {
	// DO NOT use SW_HIDE or move off-screen, as DWM drops the client surface.
	// Instead, apply Layered attributes to make it 99.6% transparent (Alpha=1).
	// PrintWindow captures the unblended frame perfectly.
	gwlExStyle := int32(GWL_EXSTYLE)
	style, _, _ := getWindowLong.Call(hwnd, uintptr(gwlExStyle))
	setWindowLong.Call(hwnd, uintptr(gwlExStyle), style|WS_EX_LAYERED|WS_EX_TOOLWINDOW)
	setLayeredWindowAttributes.Call(hwnd, 0, 1, LWA_ALPHA)
}

func CaptureWindow(hwnd uintptr) (*image.RGBA, error) {
	rdpLogger.Println("CaptureWindow entered")
	rdpLogger.Printf("HWND: %v", hwnd)

	var r rect
	resC, _, _ := getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if resC == 0 {
		return nil, fmt.Errorf("GetClientRect failed")
	}

	width := int(r.Right - r.Left)
	height := int(r.Bottom - r.Top)
	
	rdpLogger.Printf("Width: %d, Height: %d", width, height)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid window dimensions: %dx%d", width, height)
	}

	hdcWindow, _, _ := getWindowDC.Call(hwnd)
	rdpLogger.Printf("GetWindowDC succeeded: %v", hdcWindow != 0)
	if hdcWindow == 0 {
		return nil, fmt.Errorf("GetWindowDC failed")
	}
	defer releaseDC.Call(hwnd, hdcWindow)

	hdcMem, _, _ := createCompatibleDC.Call(hdcWindow)
	rdpLogger.Printf("CreateCompatibleDC succeeded: %v", hdcMem != 0)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer deleteDC.Call(hdcMem)

	hbm, _, _ := createCompatibleBitmap.Call(hdcWindow, uintptr(width), uintptr(height))
	rdpLogger.Printf("CreateCompatibleBitmap succeeded: %v", hbm != 0)
	if hbm == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer deleteObject.Call(hbm)

	oldObj, _, _ := selectObject.Call(hdcMem, hbm)
	rdpLogger.Printf("SelectObject succeeded: %v", oldObj != 0)

	// PW_RENDERFULLCONTENT = 0x00000002 (Captures DWM hardware accelerated content like hidden windows)
	res, _, errCode := printWindow.Call(hwnd, hdcMem, 0x00000002)
	rdpLogger.Printf("PrintWindow return value: %v", res)
	if res == 0 {
		rdpLogger.Printf("GetLastError(): %v", errCode)
		rdpLogger.Println("Fallback PrintWindow() is executed: true")
		// Fallback to normal PrintWindow
		res, _, errCode = printWindow.Call(hwnd, hdcMem, 0)
		rdpLogger.Printf("Result of fallback PrintWindow(): %v", res)
		if res == 0 {
			rdpLogger.Printf("Fallback GetLastError(): %v", errCode)
			return nil, fmt.Errorf("PrintWindow failed. GetLastError: %v", errCode)
		}
	} else {
		rdpLogger.Println("Fallback PrintWindow() is executed: false")
	}

	var bi bitmapInfo
	bi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bi.BmiHeader))
	bi.BmiHeader.BiWidth = int32(width)
	bi.BmiHeader.BiHeight = int32(-height) // Top-down
	bi.BmiHeader.BiPlanes = 1
	bi.BmiHeader.BiBitCount = 32
	bi.BmiHeader.BiCompression = 0 // BI_RGB

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	linesCopied, _, errCode := getDIBits.Call(hdcMem, hbm, 0, uintptr(height), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&bi)), 0)
	rdpLogger.Printf("GetDIBits return value: %v", linesCopied)
	rdpLogger.Printf("Number of scanlines copied: %v", linesCopied)
	rdpLogger.Printf("GetDIBits Width: %d", width)
	rdpLogger.Printf("GetDIBits Height: %d", height)
	rdpLogger.Printf("Bitmap format: BI_RGB (32-bit)")

	if linesCopied == 0 || int(linesCopied) != height {
		rdpLogger.Printf("Win32 error: %v", errCode)
		return nil, fmt.Errorf("GetDIBits failed or returned incomplete scanlines. Copied: %v, Expected: %v, GetLastError: %v", linesCopied, height, errCode)
	}

	selectObject.Call(hdcMem, oldObj)

	// Invert BGRA to RGBA and perform Pixel Diagnostics
	nonBlackCount := 0
	nonTransparentCount := 0
	opaqueCount := 0

	for i := 0; i < len(img.Pix); i += 4 {
		b, g, r, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
		
		img.Pix[i], img.Pix[i+2] = r, b // Swap

		if r > 0 || g > 0 || b > 0 {
			nonBlackCount++
		}
		if a == 0 {
			nonTransparentCount++ // Actually transparent count
		} else if a == 255 {
			opaqueCount++
		}
	}

	rdpLogger.Printf("Non-black pixels: %d", nonBlackCount)
	rdpLogger.Printf("Transparent pixels: %d", nonTransparentCount)
	rdpLogger.Printf("Opaque pixels: %d", opaqueCount)

	if nonBlackCount == 0 {
		return nil, fmt.Errorf("bitmap validation failed: Frame is 100%% black")
	}

	// rdpLogger.Printf("[Diagnostics] Capture Success | Size: %dx%d | Lines: %d | NonBlack: %d", width, height, linesCopied, nonBlackCount)

	return img, nil
}
