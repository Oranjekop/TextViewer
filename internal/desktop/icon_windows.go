package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"runtime"
	"sync"
	"unsafe"

	"github.com/Oranjekop/TextViewer/assets"
	"github.com/egoist/mygo"
	"golang.org/x/image/draw"
	"golang.org/x/sys/windows"
)

var (
	iconUser32       = windows.NewLazySystemDLL("user32.dll")
	getWindowDPI     = iconUser32.NewProc("GetDpiForWindow")
	iconMetrics      = iconUser32.NewProc("GetSystemMetricsForDpi")
	loadIconImage    = iconUser32.NewProc("LoadImageW")
	createIconImage  = iconUser32.NewProc("CreateIconFromResourceEx")
	setIconMessage   = iconUser32.NewProc("SendMessageW")
	destroyIconImage = iconUser32.NewProc("DestroyIcon")
	iconModule       = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
	iconSource       = sync.OnceValues(func() (image.Image, error) {
		return png.Decode(bytes.NewReader(assets.AppIcon))
	})
)

type windowIcons [2]uintptr

func configureWindowIcon(window *mygo.Window) error {
	dpi := windowDPI(window)
	icons, err := iconsForDPI(dpi)
	if err != nil {
		return err
	}
	icons.apply(window)
	window.OnClosed(func() { icons.release() })
	refresh := func() {
		current := windowDPI(window)
		if current == 0 || current == dpi {
			return
		}
		next, err := iconsForDPI(current)
		if err != nil {
			log.Printf("TextViewer: refresh window icon: %v", err)
			return
		}
		next.apply(window)
		icons.release()
		icons = next
		dpi = current
	}
	window.OnResize(refresh)
	window.OnMove(refresh)
	return nil
}

func iconsForDPI(dpi uint32) (windowIcons, error) {
	var icons windowIcons
	for i, metric := range []uintptr{49, 11} { // SM_CXSMICON, SM_CXICON.
		size, _, _ := iconMetrics.Call(metric, uintptr(dpi))
		if size == 0 || size > 1024 {
			icons.release()
			return windowIcons{}, fmt.Errorf("invalid window icon size at DPI %d", dpi)
		}
		icon, err := loadWindowIcon(int(size))
		if err != nil {
			icons.release()
			return windowIcons{}, err
		}
		icons[i] = icon
	}
	return icons, nil
}

func loadWindowIcon(size int) (uintptr, error) {
	module, _, _ := iconModule.Call(0)
	// MyGo builds icon group 1 with prefiltered 16-256px frames. Loading
	// those frames preserves the artwork that Explorer uses for the EXE;
	// scaling a raw 256px PNG with CreateIconFromResourceEx loses detail.
	icon, _, _ := loadIconImage.Call(module, 1, 1, uintptr(size), uintptr(size), 0)
	if icon != 0 {
		return icon, nil
	}
	// Plain "go run" has no icon resources. Prefilter its PNG ourselves so
	// Windows only decodes an image that is already at the target size.
	source, err := iconSource()
	if err != nil {
		return 0, err
	}
	resized := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(resized, resized.Bounds(), source, source.Bounds(), draw.Src, nil)
	var data bytes.Buffer
	if err := png.Encode(&data, resized); err != nil {
		return 0, err
	}
	pixels := data.Bytes()
	icon, _, err = createIconImage.Call(uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(len(pixels)), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	runtime.KeepAlive(pixels)
	if icon == 0 {
		return 0, fmt.Errorf("create %dpx window icon: %v", size, err)
	}
	return icon, nil
}

func (icons windowIcons) apply(window *mygo.Window) {
	const wmSetIcon = 0x0080
	for kind, icon := range icons {
		setIconMessage.Call(window.NativeHandle(), wmSetIcon, uintptr(kind), icon)
	}
}

func (icons *windowIcons) release() {
	for i, icon := range icons {
		if icon != 0 {
			destroyIconImage.Call(icon)
			icons[i] = 0
		}
	}
}

func windowDPI(window *mygo.Window) uint32 {
	dpi, _, _ := getWindowDPI.Call(window.NativeHandle())
	return uint32(dpi)
}
