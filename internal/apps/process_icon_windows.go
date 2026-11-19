package apps

import (
	"bytes"
	"image"
	"image/png"
	"unsafe"

	"golang.org/x/sys/windows"
)

var extractIcon = windows.NewLazySystemDLL("shell32.dll").NewProc("ExtractIconExW")
var iconUser32 = windows.NewLazySystemDLL("user32.dll")
var iconGDI = windows.NewLazySystemDLL("gdi32.dll")

func nativeIcon(executable string) []byte {
	path, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return nil
	}
	var hicon uintptr
	n, _, _ := extractIcon.Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&hicon)), 0, 1)
	if n != 1 || hicon == 0 {
		return nil
	}
	defer iconUser32.NewProc("DestroyIcon").Call(hicon)
	const dim = 32
	dc, _, _ := iconGDI.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		return nil
	}
	defer iconGDI.NewProc("DeleteDC").Call(dc)
	header := struct {
		Size          uint32
		Width, Height int32
		Planes, Bits  uint16
		Rest          [6]uint32
	}{Size: 40, Width: dim, Height: -dim, Planes: 1, Bits: 32}
	var pixels unsafe.Pointer
	bmp, _, _ := iconGDI.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bmp == 0 || pixels == nil {
		return nil
	}
	defer iconGDI.NewProc("DeleteObject").Call(bmp)
	selectObject := iconGDI.NewProc("SelectObject")
	old, _, _ := selectObject.Call(dc, bmp)
	defer selectObject.Call(dc, old)
	draw := iconUser32.NewProc("DrawIconEx")
	flush := iconGDI.NewProc("GdiFlush")
	data := unsafe.Slice((*byte)(pixels), dim*dim*4)
	clear(data)
	ok, _, _ := draw.Call(dc, 0, 0, hicon, dim, dim, 0, 0, 3)
	if ok == 0 {
		return nil
	}
	flush.Call()
	dark := append([]byte(nil), data...)
	// Draw on white too, to recover transparency from both modern alpha
	// icons and older AND/XOR-mask icons.
	for i := range data {
		data[i] = 255
	}
	draw.Call(dc, 0, 0, hicon, dim, dim, 0, 0, 3)
	flush.Call()
	img := image.NewRGBA(image.Rect(0, 0, dim, dim))
	for i := 0; i < len(data); i += 4 {
		a := 255 - max(0, int(data[i])-int(dark[i]), int(data[i+1])-int(dark[i+1]), int(data[i+2])-int(dark[i+2]))
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = min(dark[i+2], byte(a)), min(dark[i+1], byte(a)), min(dark[i], byte(a)), byte(a)
	}
	var b bytes.Buffer
	if png.Encode(&b, img) != nil {
		return nil
	}
	return b.Bytes()
}
