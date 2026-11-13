package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestRecentFilesOpenReorderPersistAndClear(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	first := desktopFile(t, "中文 & first.txt", "first")
	second := desktopFile(t, "second.json", "{}")
	a.openFiles([]string{first, second})
	first, second = a.tabs[0].doc.Path, a.tabs[1].doc.Path
	if !slices.Equal(a.settings.RecentFiles, []string{second, first}) {
		t.Fatalf("recent order: %v", a.settings.RecentFiles)
	}
	restarted := New()
	if !slices.Equal(restarted.settings.RecentFiles, a.settings.RecentFiles) {
		t.Fatal("history did not survive restart")
	}
	tester := ui.NewTester(a.view, 760, 420)
	if err := tester.Click(locales.File()); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem(locales.RecentFiles(), recentFileLabel(1, first)); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 2 || a.doc.Path != first || !slices.Equal(a.settings.RecentFiles, []string{first, second}) {
		t.Fatal("recent entry did not reuse and promote the existing tab")
	}
	before := a.settings
	if err := tester.Click(locales.File()); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem(locales.RecentFiles(), locales.ClearRecentFiles()); err != nil {
		t.Fatal(err)
	}
	if len(a.settings.RecentFiles) != 0 || len(New().settings.RecentFiles) != 0 || len(a.tabs) != 2 {
		t.Fatal("clear did not persist or closed documents")
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("clearing history removed a file", err)
		}
	}
	if got := loadPreferences(); got.FontSize != before.FontSize || got.Theme != before.Theme || got.Wrap != before.Wrap {
		t.Fatal("history changed other preferences")
	}
}

func TestRecentFilesBoundedAndFailedOpenNotRecorded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	for index := range recentFileLimit + 2 {
		a.openFiles([]string{desktopFile(t, fmt.Sprintf("%d.txt", index), "text")})
	}
	if len(a.settings.RecentFiles) != recentFileLimit {
		t.Fatal("history exceeded limit")
	}
	before := slices.Clone(a.settings.RecentFiles)
	a.openFiles([]string{filepath.Join(t.TempDir(), "missing.txt")})
	if !slices.Equal(a.settings.RecentFiles, before) {
		t.Fatal("failed load changed recent files")
	}
	a.openFiles([]string{a.tabs[0].doc.Path})
	if len(a.settings.RecentFiles) != recentFileLimit || a.settings.RecentFiles[0] != a.tabs[0].doc.Path {
		t.Fatal("reopening an older tab did not promote it")
	}
}

func TestRecentFilesIgnoreInvalidStoredPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "valid.txt")
	p := preferences{RecentFiles: []string{"relative.txt", "", path, path, path + "\x00"}}
	if err := savePreferences(p); err != nil {
		t.Fatal(err)
	}
	if got := loadPreferences().RecentFiles; !slices.Equal(got, []string{path}) {
		t.Fatalf("invalid recent paths retained: %v", got)
	}
}

func TestRecentFilesTrackSuccessfulSaveAsOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	a.doc.Buffer.Set("new text")
	a.save(true, nil) // Cancelling the save dialog must not create a record.
	if len(a.settings.RecentFiles) != 0 {
		t.Fatal("cancelled save recorded a path")
	}
	target := filepath.Join(t.TempDir(), "saved.txt")
	a.dialogs.(*fakeDialogs).save = func(mygo.SaveDialogOptions) (string, error) { return target, nil }
	a.save(true, nil)
	if a.doc.Dirty() || !slices.Equal(a.settings.RecentFiles, []string{a.doc.Path}) {
		t.Fatal("first successful save did not record its path")
	}
	old := a.doc.Path
	target = filepath.Join(t.TempDir(), "renamed.txt")
	a.save(true, nil)
	if !slices.Equal(a.settings.RecentFiles, []string{a.doc.Path, old}) {
		t.Fatal("save as did not promote the new path")
	}
}
