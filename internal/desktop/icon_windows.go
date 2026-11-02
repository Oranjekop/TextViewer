package desktop

import (
	"log"

	"github.com/Oranjekop/TextViewer/assets"
	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

var getWindowDPI = windows.NewLazySystemDLL("user32.dll").NewProc("GetDpiForWindow")

func configureWindowIcon(window *mygo.Window) error {
	if err := window.SetIcon(assets.AppIcon); err != nil {
		return err
	}
	dpi := windowDPI(window)
	refresh := func() {
		current := windowDPI(window)
		if current == 0 || current == dpi {
			return
		}
		// Decode the original PNG again at the new DPI, never enlarge an
		// icon that was rendered for the previous monitor.
		if err := window.SetIcon(assets.AppIcon); err != nil {
			log.Printf("TextViewer: refresh window icon: %v", err)
			return
		}
		dpi = current
	}
	window.OnResize(refresh)
	window.OnMove(refresh)
	return nil
}

func windowDPI(window *mygo.Window) uint32 {
	dpi, _, _ := getWindowDPI.Call(window.NativeHandle())
	return uint32(dpi)
}
