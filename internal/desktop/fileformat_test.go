package desktop

import (
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

func TestOutputSettingsMenusMarkMetadataDirty(t *testing.T) {
	a := testApp()
	snapshot := a.doc.Buffer.Snapshot()
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("编码"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("UTF-16 LE"); err != nil {
		t.Fatal(err)
	}
	if a.doc.Encoding != document.UTF16LE || !a.doc.Dirty() || !a.doc.HasChanges() {
		t.Fatal("encoding-only change is not dirty")
	}
	if err := tester.Click("换行格式"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("Unix / Linux (LF)"); err != nil {
		t.Fatal(err)
	}
	if a.doc.Buffer.LineEnding() != "\n" || a.doc.Buffer.Version() != snapshot.Version() {
		t.Fatal("line ending selection edited the text")
	}
	if err := tester.Click("编码"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("UTF-8"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("换行格式"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("Windows (CRLF)"); err != nil {
		t.Fatal(err)
	}
	if a.doc.Dirty() || a.doc.HasChanges() || a.doc.Buffer.Snapshot() != snapshot {
		t.Fatal("restoring metadata did not restore saved state")
	}
}

func TestOutputSettingsCannotChangeDuringJob(t *testing.T) {
	a := testApp()
	a.busy = locales.Text("正在保存…")
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("编码"); err != nil {
		t.Fatal(err)
	}
	_ = tester.ChooseMenuItem("UTF-16 BE")
	if a.doc.Encoding != document.UTF8 {
		t.Fatal("encoding changed during a save")
	}
}
