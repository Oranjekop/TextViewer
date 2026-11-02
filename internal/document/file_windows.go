package document

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var replaceFileW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

func SamePath(a, b string) bool {
	a, _ = ResolvePath(a)
	b, _ = ResolvePath(b)
	return strings.EqualFold(a, b)
}

func replaceFile(source, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if _, err = os.Stat(target); os.IsNotExist(err) {
		return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
	} else if err != nil {
		return err
	}
	// ReplaceFile preserves the original file's ACL and creation metadata.
	ok, _, callErr := replaceFileW.Call(uintptr(unsafe.Pointer(to)), uintptr(unsafe.Pointer(from)), 0, 0, 0, 0)
	if ok == 0 {
		return callErr
	}
	return nil
}
