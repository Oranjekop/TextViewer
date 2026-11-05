package document

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
			d.Buffer.SetLineEnding("\r\n")
			d.Encoding = enc
			d.Buffer.Set("中文文本 123\n第二行\n")
			path := filepath.Join(t.TempDir(), "roundtrip.txt")
			snapshot := d.CaptureSave()
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
			if _, err = reloaded.SaveSnapshot(context.Background(), copyPath, reloaded.CaptureSave()); err != nil {
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
		if _, err = d.SaveSnapshot(context.Background(), copyPath, d.CaptureSave()); err != nil {
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
		if _, err := d.SaveSnapshot(context.Background(), path, d.CaptureSave()); err != nil {
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
	_, err = d.SaveSnapshot(ctx, path, d.CaptureSave())
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
	snapshot := d.CaptureSave()
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
	if _, err = d.SaveSnapshot(context.Background(), path, d.CaptureSave()); err == nil {
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
	_, err = d.SaveSnapshot(context.Background(), path, d.CaptureSave())
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
	snapshot := d.CaptureSave()
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
	if string(got) != "captured"+snapshot.lineEnding {
		t.Fatalf("saved %q", got)
	}
}

func TestNewDocumentUsesPlatformLineEnding(t *testing.T) {
	want := "\n"
	if runtime.GOOS == "windows" {
		want = "\r\n"
	}
	doc := New()
	if doc.Buffer.LineEnding() != want || doc.Dirty() || doc.HasChanges() {
		t.Fatal("new document has the wrong platform format or is dirty")
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

func TestEncodingAndLineEndingConversion(t *testing.T) {
	for _, enc := range []Encoding{UTF8, UTF8BOM, UTF16LE, UTF16BE, GB18030} {
		for _, ending := range []string{"\r\n", "\n"} {
			t.Run(string(enc)+ending, func(t *testing.T) {
				path := fixture(t, []byte("中文 😀\r\nsecond\r\n"))
				d, err := Load(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				original := d.Buffer.Snapshot()
				d.Encoding = enc
				d.Buffer.SetLineEnding(ending)
				changed := enc != UTF8 || ending != "\r\n"
				if d.Dirty() != changed || d.HasChanges() != changed {
					t.Fatalf("metadata change not tracked: dirty=%v changes=%v", d.Dirty(), d.HasChanges())
				}
				if original.Version() != d.Buffer.Version() || d.Buffer.CanUndo() {
					t.Fatal("metadata conversion rewrote the text or undo history")
				}
				snapshot := d.CaptureSave()
				stamp, err := d.SaveSnapshot(context.Background(), path, snapshot)
				if err != nil {
					t.Fatal(err)
				}
				d.AcceptSave(path, snapshot, stamp)
				reloaded, err := Load(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				if d.Dirty() || d.HasChanges() || reloaded.Encoding != enc ||
					reloaded.Buffer.LineEnding() != ending || reloaded.Buffer.String() != original.String() {
					t.Fatal("converted document did not reload with matching settings/text")
				}
			})
		}
	}
}

func TestSavedMetadataUsesCapturedValues(t *testing.T) {
	d := New()
	d.Buffer.Set("中文\nsecond\n")
	d.Encoding = UTF16LE
	d.Buffer.SetLineEnding("\n")
	snapshot := d.CaptureSave()
	d.Encoding = UTF8BOM
	d.Buffer.SetLineEnding("\r\n")
	path := filepath.Join(t.TempDir(), "captured.txt")
	stamp, err := d.SaveSnapshot(context.Background(), path, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	d.AcceptSave(path, snapshot, stamp)
	reloaded, err := Load(context.Background(), path)
	if err != nil || reloaded.Encoding != UTF16LE || reloaded.Buffer.LineEnding() != "\n" {
		t.Fatalf("save used live metadata: %+v %v", reloaded, err)
	}
	if !d.Dirty() || !d.HasChanges() {
		t.Fatal("newer output settings were incorrectly marked saved")
	}
	d.Encoding = UTF16LE
	d.Buffer.SetLineEnding("\n")
	if d.Dirty() || d.HasChanges() {
		t.Fatal("captured metadata did not become the saved baseline")
	}
}

func TestUnsupportedEncodingDoesNotOverwriteFile(t *testing.T) {
	path := fixture(t, []byte("original"))
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	d.Encoding = "unknown"
	d.Buffer.Set("replacement")
	if _, err = d.SaveSnapshot(context.Background(), path, d.CaptureSave()); err == nil {
		t.Fatal("unsupported encoding was silently written as UTF-8")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatalf("original file changed: %q %v", got, err)
	}
}

func TestInvalidUTF8AfterLargePrefixStillUsesLegacyDetection(t *testing.T) {
	content := append(bytes.Repeat([]byte("ascii\n"), 16384), 0xd6, 0xd0, 0xce, 0xc4)
	d, err := Load(context.Background(), fixture(t, content))
	if err != nil {
		t.Fatal(err)
	}
	if d.Encoding != GB18030 || !strings.HasSuffix(d.Buffer.String(), "中文") {
		t.Fatal("UTF-8 validation optimization missed a legacy suffix")
	}
}
