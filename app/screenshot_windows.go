//go:build windows

package main

import (
	"errors"
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	gdi32                        = windows.NewLazySystemDLL("gdi32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procPrintWindow              = user32.NewProc("PrintWindow")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procGetDIBits                = gdi32.NewProc("GetDIBits")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// ownWindow finds this process's largest visible top-level window.
func ownWindow() (uintptr, error) {
	pid := uint32(windows.GetCurrentProcessId())
	var best uintptr
	var bestArea int32
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		var r rect
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		if a := (r.Right - r.Left) * (r.Bottom - r.Top); a > bestArea {
			best, bestArea = hwnd, a
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if best == 0 {
		return 0, errors.New("the app's window was not found")
	}
	return best, nil
}

// captureWindow draws the window's client area, including the web view, into an image.
func captureWindow() (image.Image, error) {
	hwnd, err := ownWindow()
	if err != nil {
		return nil, err
	}
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w <= 0 || h <= 0 {
		return nil, errors.New("the window has no size")
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	// PW_CLIENTONLY (1) | PW_RENDERFULLCONTENT (2): GPU-drawn content such as the web view is included.
	ok, _, _ := procPrintWindow.Call(hwnd, mem, 3)
	procSelectObject.Call(mem, old)
	if ok == 0 {
		return nil, errors.New("PrintWindow failed")
	}
	hdr := bitmapInfoHeader{Width: w, Height: -h, Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	buf := make([]byte, int(w)*int(h)*4)
	if n, _, _ := procGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&hdr)), 0); n == 0 {
		return nil, errors.New("GetDIBits failed")
	}
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for i := 0; i < len(buf); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = buf[i+2], buf[i+1], buf[i], 255
	}
	return img, nil
}
