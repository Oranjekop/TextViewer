package document

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func fixture(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "中文 & test.txt")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEncodingRoundTrip(t *testing.T) {
	for _, enc := range []Encoding{UTF8, UTF8BOM, UTF16LE, UTF16BE, GB18030} {
		t.Run(string(enc), func(t *testing.T) {
			d := New()
			d.Encoding = enc
			d.Buffer.Set("中文文本 123\n第二行\n")
			path := filepath.Join(t.TempDir(), "roundtrip.txt")
			snapshot := d.Buffer.Snapshot()
			stamp, err := d.SaveSnapshot(context.Background(), path, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			d.AcceptSave(path, snapshot, stamp)
			if d.Dirty() {
				t.Fatal("saved document is dirty")
			}
			reloaded, err := Load(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Encoding != enc || reloaded.Buffer.String() != d.Buffer.String() {
				t.Fatalf("loaded %q %q", reloaded.Encoding, reloaded.Buffer.String())
			}
			if reloaded.Buffer.LineEnding() != "\r\n" {
				t.Fatal("CRLF was not retained")
			}
			copyPath := filepath.Join(t.TempDir(), "copy.txt")
			if _, err = reloaded.SaveSnapshot(context.Background(), copyPath, reloaded.Buffer.Snapshot()); err != nil {
				t.Fatal(err)
			}
			want, _ := os.ReadFile(path)
			got, _ := os.ReadFile(copyPath)
			if !bytes.Equal(got, want) {
				t.Fatal("unmodified roundtrip changed file bytes")
			}
		})
	}
}

func TestLFSaveAndEmptyFile(t *testing.T) {
	for _, content := range []string{"", "a\nb\n", "hello 😀\n第三行"} {
		path := fixture(t, []byte(content))
		d, err := Load(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if d.Buffer.String() != content {
			t.Fatalf("got %q", d.Buffer.String())
		}
		copyPath := filepath.Join(t.TempDir(), "save.txt")
		if _, err = d.SaveSnapshot(context.Background(), copyPath, d.Buffer.Snapshot()); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(copyPath)
		if string(got) != content {
			t.Fatalf("saved %q", got)
		}
	}
}

func TestRejectBinaryAndInvalidUTF8BOM(t *testing.T) {
	for _, data := range [][]byte{
		{0, 1, 2, 0},
		{0xef, 0xbb, 0xbf, 0xff, 0xff},
		{0xff, 0xfe, 0x00, 0xd8},
		{0xfe, 0xff, 0x00},
	} {
		if _, err := Load(context.Background(), fixture(t, data)); err == nil {
			t.Fatal("expected invalid text to be rejected")
		}
	}
}

func TestLiteralReplacementCharacterIsNotCorruption(t *testing.T) {
	for _, enc := range []Encoding{UTF8, UTF16LE, GB18030} {
		d := New()
		d.Encoding = enc
		d.Buffer.Set("中文 \ufffd")
		path := filepath.Join(t.TempDir(), "replacement.txt")
		if _, err := d.SaveSnapshot(context.Background(), path, d.Buffer.Snapshot()); err != nil {
			t.Fatal(err)
		}
		reloaded, err := Load(context.Background(), path)
		if err != nil || reloaded.Buffer.String() != d.Buffer.String() {
			t.Fatalf("%s: %v", enc, err)
		}
	}
}

func TestCancelledLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Load(ctx, fixture(t, []byte("hello")))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestSaveCancellationPreservesFile(t *testing.T) {
	path := fixture(t, []byte("original"))
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("replacement")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = d.SaveSnapshot(ctx, path, d.Buffer.Snapshot())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatal("cancelled save overwrote original")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary save file was not cleaned up")
	}
}

func TestSavingExistingFileReplacesContents(t *testing.T) {
	path := fixture(t, []byte("original\r\n"))
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("replacement\n中文\n")
	snapshot := d.Buffer.Snapshot()
	stamp, err := d.SaveSnapshot(context.Background(), path, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	d.AcceptSave(path, snapshot, stamp)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "replacement\r\n中文\r\n" {
		t.Fatalf("saved %q: %v", got, err)
	}
	if d.Dirty() || d.HasChanges() || d.DiskChanged() {
		t.Fatal("successful replacement left saved state inconsistent")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("successful replacement left a temporary file")
	}
}

func TestReadOnlySaveDoesNotLeaveTemporaryFiles(t *testing.T) {
	path := fixture(t, []byte("original"))
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("changed")
	if _, err = d.SaveSnapshot(context.Background(), path, d.Buffer.Snapshot()); err == nil {
		t.Fatal("read-only file should require Save As")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatal("read-only original changed")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("read-only save left a temporary file")
	}
}

func TestExternalModificationIsNotOverwritten(t *testing.T) {
	path := fixture(t, []byte("original"))
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Set("my changes")
	if err = os.WriteFile(path, []byte("a different program changed this file"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = d.SaveSnapshot(context.Background(), path, d.Buffer.Snapshot())
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "a different program changed this file" {
		t.Fatal("external change was overwritten")
	}
}

func TestCapturedSaveDoesNotMarkNewEditsSaved(t *testing.T) {
	d := New()
	d.Buffer.Set("captured\n")
	snapshot := d.Buffer.Snapshot()
	d.Buffer.Set("newer edits\n")
	path := filepath.Join(t.TempDir(), "save.txt")
	stamp, err := d.SaveSnapshot(context.Background(), path, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	d.AcceptSave(path, snapshot, stamp)
	if !d.Dirty() || !d.HasChanges() {
		t.Fatal("edits newer than the saved snapshot were lost")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "captured\r\n" {
		t.Fatalf("saved %q", got)
	}
}

func TestUndoToSavedContent(t *testing.T) {
	d, err := Load(context.Background(), fixture(t, []byte(strings.Repeat("line\n", 10000))))
	if err != nil {
		t.Fatal(err)
	}
	d.Buffer.Edit(editor.Edit{Start: 0, End: 0, Text: "X"})
	if !d.HasChanges() {
		t.Fatal("edit not detected")
	}
	d.Buffer.Undo()
	if d.HasChanges() {
		t.Fatal("undo to saved content still requests save")
	}
}
