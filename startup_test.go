package main

import (
	"reflect"
	"testing"
)

func TestFileOpenWaitsForWindowAndPreservesEveryPath(t *testing.T) {
	var callbacks []func()
	windowReady := false
	// The startup callback is registered before the native open handler.
	callbacks = append(callbacks, func() { windowReady = true })
	var opened []string
	handle := fileOpenHandler(func(fn func()) {
		callbacks = append(callbacks, fn)
	}, func(paths []string, dir string) {
		if !windowReady {
			t.Fatal("file reached the editor before its window existed")
		}
		if dir != "" {
			t.Fatalf("unexpected working directory: %q", dir)
		}
		opened = append(opened, paths...)
	})
	paths := []string{"/tmp/中文 文件.txt", "/tmp/second.md"}
	for _, path := range paths {
		handle(path)
	}
	if len(opened) != 0 {
		t.Fatal("cold-start file request was dispatched before readiness")
	}
	for _, fn := range callbacks {
		fn()
	}
	if !reflect.DeepEqual(opened, paths) {
		t.Fatalf("opened %q, want %q", opened, paths)
	}
}

func TestFileOpenAfterReady(t *testing.T) {
	var opened []string
	handle := fileOpenHandler(func(fn func()) { fn() }, func(paths []string, _ string) {
		opened = append(opened, paths...)
	})
	handle("/tmp/already-running.txt")
	if !reflect.DeepEqual(opened, []string{"/tmp/already-running.txt"}) {
		t.Fatalf("file was lost after readiness: %q", opened)
	}
}
