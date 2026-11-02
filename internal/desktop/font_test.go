package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/egoist/mygo/ui"
)

func TestFontChoiceSurvivesRestartAndDocumentChanges(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	a.doc.Buffer.Set("original")
	a.fontDraft = "Consolas"
	a.applyFont()
	if prefs := loadPreferences(); prefs.FontFamily != "Consolas" {
		t.Fatalf("font was not saved: %+v", prefs)
	}
	restarted := New()
	for _, path := range []string{"notes.txt", "example.json", "main.go", ""} {
		restarted.doc = document.New()
		restarted.doc.Path = path
		restarted.resetEditor()
		tester := ui.NewTester(restarted.view, 760, 600)
		tester.Frame()
		if restarted.state.Font.Family != "Consolas" || restarted.doc.Dirty() {
			t.Fatalf("%s: font=%q dirty=%v", path, restarted.state.Font.Family, restarted.doc.Dirty())
		}
	}
	if a.doc.Buffer.String() != "original" {
		t.Fatal("font preference changed the document")
	}
}

func TestFontDialogSearchApplyAndCancel(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("字体"); err != nil {
		t.Fatal(err)
	}
	if !a.fontDialog {
		t.Fatal("font dialog did not open")
	}
	r, ok := tester.Find("字体名称")
	if !ok || r.W < 300 || r.X < 0 || r.X+r.W > 760 {
		t.Fatalf("font picker does not fit: %+v", r)
	}
	if err := tester.Click("字体名称"); err != nil {
		t.Fatal(err)
	}
	tester.Type("Consolas")
	tester.Key(0, ui.KeyEnter)
	if a.fontDraft != "Consolas" {
		t.Fatalf("search did not choose the system font: %q", a.fontDraft)
	}
	if err := tester.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if a.fontDialog || a.settings.FontFamily != "Consolas" || loadPreferences().FontFamily != "Consolas" {
		t.Fatal("applying font did not persist or dismiss")
	}
	if err := tester.Click("字体"); err != nil {
		t.Fatal(err)
	}
	a.fontDraft = fontAutomatic
	if err := tester.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.settings.FontFamily != "Consolas" {
		t.Fatal("cancel changed the font")
	}
}

func TestOldFontPreferencesAndAutomaticMode(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := preferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"fontSize":20,"wrap":false,"theme":"深色"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a := New()
	if a.settings.FontFamily != "" || a.fontSize != "20" || a.wrap || a.settings.Theme != "深色" {
		t.Fatal("old preferences were not retained")
	}
	if a.editorFont() != "Microsoft YaHei UI" {
		t.Fatal("plain text automatic font changed")
	}
	a.doc.Path = "data.json"
	a.resetEditor()
	if a.editorFont() != "monospace" {
		t.Fatal("code automatic font changed")
	}
	a.settings.FontFamily = "Consolas"
	a.fontDraft = fontAutomatic
	a.applyFont()
	restarted := New()
	if restarted.settings.FontFamily != "" || restarted.editorFont() != "Microsoft YaHei UI" {
		t.Fatal("automatic font choice did not survive restart")
	}
}

func TestFontDialogPreview(t *testing.T) {
	output := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if output == "" {
		t.Skip("set TEXTVIEWER_PREVIEW_DIR to render the font dialog")
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"浅色", "深色"} {
		a := testApp()
		a.settings.Theme = theme
		a.chooseFont()
		a.fontDraft = "Consolas"
		tester := ui.NewTester(a.view, 760, 600)
		name := "font-dialog-light.png"
		if theme == "深色" {
			name = "font-dialog-dark.png"
		}
		f, err := os.Create(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tester.Image())
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("save preview: %v %v", err, closeErr)
		}
	}
}
