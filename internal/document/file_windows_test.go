package document

import (
	"context"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestReplacingExistingFilePreservesCreationTime(t *testing.T) {
	path := fixture(t, []byte("original"))
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	created := windows.NsecToFiletime(time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC).UnixNano())
	err = windows.SetFileTime(windows.Handle(f.Fd()), &created, nil, nil)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("replacement")
	if _, err = d.SaveSnapshot(context.Background(), path, d.Buffer.Snapshot()); err != nil {
		t.Fatal(err)
	}
	f, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var after windows.Filetime
	if err = windows.GetFileTime(windows.Handle(f.Fd()), &after, nil, nil); err != nil {
		t.Fatal(err)
	}
	if after != created {
		t.Fatal("replacing the file lost its original creation time")
	}
}
