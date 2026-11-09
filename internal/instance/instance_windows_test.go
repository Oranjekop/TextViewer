package instance

import (
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestClaimAfterPreviousEditorExits(t *testing.T) {
	name := "TextViewer.Test." + rand.Text()
	mutexName, _ := windows.UTF16PtrFromString(`Local\` + name)
	class, _ := windows.UTF16PtrFromString(name)
	old, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil {
		t.Fatal(err)
	}
	// Model the old editor's final shutdown phase: the activation window
	// is already gone, but its mutex handle has not yet been closed.
	released := make(chan error, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		released <- windows.CloseHandle(old)
	}()
	owner, forwarded, err := claimOrForward(mutexName, class)
	if closeErr := <-released; closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || forwarded || owner == 0 {
		t.Fatalf("restart did not take ownership: handle=%v forwarded=%v error=%v", owner, forwarded, err)
	}
	windows.CloseHandle(owner)
}
