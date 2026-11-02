package desktop

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowIconHandlesAreOwnedAndReleased(t *testing.T) {
	getInfo := iconUser32.NewProc("GetIconInfo")
	deleteBitmap := windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject")
	for _, dpi := range []uint32{96, 144, 192, 240, 288} {
		icons, err := iconsForDPI(dpi)
		if err != nil {
			t.Fatal(err)
		}
		handles := icons
		if handles[0] == 0 || handles[1] == 0 || handles[0] == handles[1] {
			icons.release()
			t.Fatalf("DPI %d did not create two independent native icons", dpi)
		}
		icons.release()
		if icons != (windowIcons{}) {
			t.Fatal("released icons retained stale handles")
		}
		for _, handle := range handles {
			var info struct {
				Icon, HotspotX, HotspotY uint32
				Mask, Color              uintptr
			}
			if result, _, _ := getInfo.Call(handle, uintptr(unsafe.Pointer(&info))); result != 0 {
				deleteBitmap.Call(info.Mask)
				deleteBitmap.Call(info.Color)
				t.Fatal("release left a native icon allocated")
			}
		}
	}
}
