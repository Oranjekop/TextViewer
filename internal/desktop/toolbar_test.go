package desktop

import (
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestToolbarIconsAreValidSVGs(t *testing.T) {
	if len(toolbarIcons) != 17 {
		t.Fatalf("got %d icons", len(toolbarIcons))
	}
	for name, icon := range toolbarIcons {
		w, h := icon.Size()
		if w != 24 || h != 24 {
			t.Fatalf("%s has an inconsistent view box: %v x %v", name, w, h)
		}
		tester := ui.NewTester(func(c *ui.Context) { ui.Icon(c, icon).Size(24, 24) }, 24, 24)
		img := tester.Image()
		drawn := false
		for y := range 24 {
			for x := range 24 {
				c := img.RGBAAt(x, y)
				drawn = drawn || c.A != 0 && c.R < 150 && c.G < 150 && c.B < 150
			}
		}
		if !drawn {
			t.Fatalf("%s rendered no visible strokes", name)
		}
	}
}

func TestToolbarUndoRedoTracksDocumentHistory(t *testing.T) {
	a := testApp()
	tester := ui.NewTester(a.view, 760, 600)
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(locales.Undo())
	click(locales.Redo())
	if a.doc.Buffer.CanUndo() || a.doc.Buffer.CanRedo() || a.doc.Buffer.String() != "" {
		t.Fatal("empty history buttons changed the document")
	}
	click(locales.EditorArea())
	tester.Type("中文😀")
	if a.doc.Buffer.String() != "中文😀" {
		t.Fatalf("typing fixture: %q", a.doc.Buffer.String())
	}
	click(locales.Undo())
	if a.doc.Buffer.String() != "" || !tester.Focused(locales.EditorArea()) {
		t.Fatalf("toolbar undo: text=%q, editor focused=%v", a.doc.Buffer.String(), tester.Focused(locales.EditorArea()))
	}
	click(locales.Redo())
	if a.doc.Buffer.String() != "中文😀" || !tester.Focused(locales.EditorArea()) {
		t.Fatal("toolbar redo did not restore the edit and editor focus")
	}
	click(locales.Undo())
	tester.Type("new edit")
	click(locales.Redo())
	if a.doc.Buffer.String() != "new edit" || a.doc.Buffer.CanRedo() {
		t.Fatal("new editing did not discard the old redo branch")
	}
	first := a.documentTab
	a.addDocument(document.New())
	second := a.documentTab
	tester.Frame()
	click(locales.Undo())
	click(locales.Redo())
	if second.doc.Buffer.String() != "" || first.doc.Buffer.String() != "new edit" {
		t.Fatal("empty tab used another tab's history")
	}
	a.activateTab(first)
	tester.Frame()
	for _, confirming := range []bool{false, true} {
		a.confirming = confirming
		if !confirming {
			a.busy = locales.EditingText
		} else {
			a.busy = nil
		}
		tester.Frame()
		click(locales.Undo())
		if first.doc.Buffer.String() != "new edit" {
			t.Fatal("locked toolbar accepted undo")
		}
	}
	a.confirming, a.busy = false, nil
	tester.Frame()
	click(locales.Undo())
	if first.doc.Buffer.String() != "" || second.doc.Buffer.String() != "" {
		t.Fatal("switching tabs did not restore the active document history")
	}
	click(locales.Redo())
	if first.doc.Buffer.String() != "new edit" || second.doc.Buffer.String() != "" {
		t.Fatal("redo changed the inactive document")
	}
}

func TestToolbarTooltipsAndIconActions(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	tester := ui.NewTester(a.view, 760, 600)
	r, ok := tester.Find("打开")
	if !ok {
		t.Fatal("icon has no accessible label")
	}
	tester.Move(r.X+r.W/2, r.Y+r.H/2)
	tester.Advance(650 * time.Millisecond)
	tester.Frame()
	if !tester.HasText("打开 (Ctrl+O)") {
		t.Fatal("open icon tooltip absent")
	}
	if err := tester.Click("查找"); err != nil {
		t.Fatal(err)
	}
	if !a.findOpen || a.replacing {
		t.Fatal("find icon did not open the find bar")
	}
	if err := tester.Click("替换"); err != nil {
		t.Fatal(err)
	}
	if !a.replacing {
		t.Fatal("replace icon did not open replacement controls")
	}
	if err := tester.Click("自动换行"); err != nil {
		t.Fatal(err)
	}
	if a.wrap || a.state.SoftWrap {
		t.Fatal("wrap toggle did not turn off")
	}
	tester.Key(ui.Alt, ui.KeyZ)
	if !a.wrap || !a.state.SoftWrap {
		t.Fatal("Alt+Z did not turn wrap on")
	}
}

func TestToolbarFitsAllLanguageNames(t *testing.T) {
	for _, theme := range []string{"浅色", "深色"} {
		a := testApp()
		a.settings.Theme = theme
		tester := ui.NewTester(a.view, 760, 600)
		for _, language := range editor.Languages() {
			a.setLanguage(language.Name)
			tester.Frame()
			r, ok := tester.Find("代码格式")
			settings, settingsOK := tester.Find("设置")
			if !ok || !settingsOK || r.W < 164 || r.X+r.W > settings.X {
				t.Fatalf("%s %s: format overlaps settings", theme, language.Name)
			}
			if !tester.HasText(language.Name) {
				t.Fatalf("%s %s: current language not shown", theme, language.Name)
			}
		}
	}
}
