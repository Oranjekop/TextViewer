package shellbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func fixtureExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	// Append validates a real Windows PE, as used by the release pipeline.
	if !bytes.HasPrefix(data, []byte("MZ")) {
		t.Skip("PE bundling test requires Windows")
	}
	directory := t.TempDir()
	for _, name := range Files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("embedded fixture "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, "TextViewer.exe")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, directory); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSingleExecutableCanMoveAndPrepareWithoutSidecars(t *testing.T) {
	executable := fixtureExecutable(t)
	files, err := Read(executable)
	if err != nil {
		t.Fatal(err)
	}
	// Remove all build resources: only the EXE may be used from here on.
	for _, name := range Files {
		if err := os.Remove(filepath.Join(filepath.Dir(executable), name)); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(t.TempDir(), "中文 integration cache")
	directory, err := Directory(executable, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("locating the bundle created a cache directory")
	}
	var wait sync.WaitGroup
	for range 2 {
		wait.Go(func() {
			prepared, err := Prepare(executable, root)
			if err != nil || prepared != directory {
				t.Errorf("concurrent preparation: %s %v", prepared, err)
			}
		})
	}
	wait.Wait()
	for name, expected := range files {
		actual, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("missing or changed %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "TextViewerShell.dll"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(executable, root); err == nil {
		t.Fatal("accepted a modified immutable DLL")
	}
}

func TestBundleRejectsRepackingAndMissingResources(t *testing.T) {
	executable := fixtureExecutable(t)
	if err := Append(executable, filepath.Dir(executable)); err == nil {
		t.Fatal("appended another payload to an already bundled executable")
	}
	if _, err := Read(filepath.Join(t.TempDir(), "absent.exe")); err == nil {
		t.Fatal("accepted an absent executable")
	}
}
