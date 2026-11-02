// Package instance forwards launches using Windows' message loop. A named
// mutex elects the owner immediately, without probing a stale Unix socket.
package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	registerClass     = user32.NewProc("RegisterClassExW")
	unregisterClass   = user32.NewProc("UnregisterClassW")
	createWindow      = user32.NewProc("CreateWindowExW")
	destroyWindow     = user32.NewProc("DestroyWindow")
	findWindow        = user32.NewProc("FindWindowExW")
	defaultWindowProc = user32.NewProc("DefWindowProcW")
	sendMessage       = user32.NewProc("SendMessageTimeoutW")
	windowProcess     = user32.NewProc("GetWindowThreadProcessId")
	allowForeground   = user32.NewProc("AllowSetForegroundWindow")
	moduleHandle      = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	wmCopyData = 0x004a
	messageID  = 0x54565731
	maxMessage = 1 << 20
)

type message struct {
	Args      []string
	Directory string
}

type copyData struct {
	ID   uintptr
	Size uint32
	Data unsafe.Pointer
}

type windowClass struct {
	Size, Style                      uint32
	Procedure                        uintptr
	ClassExtra, WindowExtra          int32
	Module, Icon, Cursor, Background uintptr
	Menu, Name                       *uint16
	SmallIcon                        uintptr
}

type Lock struct {
	mutex          windows.Handle
	window, module uintptr
	class          *uint16
}

// Start runs on the UI thread. It reports forwarded only after the existing
// editor accepts the complete launch, including an empty Start-menu launch.
func Start(open func([]string, string)) (*Lock, bool, error) {
	if os.Getenv("MYGO_GENERATE") != "" {
		return &Lock{}, false, nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, false, err
	}
	// XDG_RUNTIME_DIR keeps the native regression harness isolated as well.
	digest := sha256.Sum256([]byte(config + "\n" + os.Getenv("XDG_RUNTIME_DIR")))
	name := "TextViewer." + hex.EncodeToString(digest[:12])
	mutexName, _ := windows.UTF16PtrFromString(`Local\` + name)
	mutex, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, false, err
	}
	class, _ := windows.UTF16PtrFromString(name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(mutex)
		forwarded, err := forward(class)
		return nil, forwarded, err
	}
	lock := &Lock{mutex: mutex, class: class}
	lock.module, _, _ = moduleHandle.Call(0)
	procedure := windows.NewCallback(func(hwnd uintptr, msg uint32, wparam uintptr, lparam unsafe.Pointer) uintptr {
		if msg == wmCopyData && lparam != nil {
			data := (*copyData)(lparam)
			if data.ID != messageID || data.Size == 0 || data.Size > maxMessage || data.Data == nil {
				return 0
			}
			var request message
			if json.Unmarshal(unsafe.Slice((*byte)(data.Data), int(data.Size)), &request) != nil {
				return 0
			}
			open(request.Args, request.Directory)
			return 1
		}
		result, _, _ := defaultWindowProc.Call(hwnd, uintptr(msg), wparam, uintptr(lparam))
		return result
	})
	definition := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Procedure: procedure, Module: lock.module, Name: class}
	if result, _, err := registerClass.Call(uintptr(unsafe.Pointer(&definition))); result == 0 {
		lock.Close()
		return nil, false, fmt.Errorf("register activation window: %w", err)
	}
	// HWND_MESSAGE: no visible window, taskbar entry, or shell activation.
	lock.window, _, err = createWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, lock.module, 0)
	if lock.window == 0 {
		lock.Close()
		return nil, false, fmt.Errorf("create activation window: %w", err)
	}
	return lock, false, nil
}

func forward(class *uint16) (bool, error) {
	directory, _ := os.Getwd()
	bytes, err := json.Marshal(message{os.Args[1:], directory})
	if err != nil || len(bytes) > maxMessage {
		return false, errors.New("activation request is too large")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		window, _, _ := findWindow.Call(^uintptr(2), 0, uintptr(unsafe.Pointer(class)), 0)
		if window != 0 {
			var pid uint32
			windowProcess.Call(window, uintptr(unsafe.Pointer(&pid)))
			allowForeground.Call(uintptr(pid))
			data := copyData{messageID, uint32(len(bytes)), unsafe.Pointer(&bytes[0])}
			var accepted uintptr
			result, _, err := sendMessage.Call(window, wmCopyData, 0, uintptr(unsafe.Pointer(&data)), 0x0002, 5000, uintptr(unsafe.Pointer(&accepted)))
			runtime.KeepAlive(bytes)
			if result != 0 && accepted == 1 {
				return true, nil
			}
			return false, fmt.Errorf("the existing editor did not accept the launch: %v", err)
		}
		if time.Now().After(deadline) {
			return false, errors.New("the existing editor did not create its activation window")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (lock *Lock) Close() {
	if lock == nil {
		return
	}
	if lock.window != 0 {
		destroyWindow.Call(lock.window)
		lock.window = 0
	}
	if lock.class != nil {
		unregisterClass.Call(uintptr(unsafe.Pointer(lock.class)), lock.module)
		lock.class = nil
	}
	if lock.mutex != 0 {
		windows.CloseHandle(lock.mutex)
		lock.mutex = 0
	}
}
