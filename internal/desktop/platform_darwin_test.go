package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMacCommandShortcutsAndControlTab(t *testing.T) {
	a := testApp()
	tester := ui.NewTester(a.view, 760, 600)
	tester.Type("one two one")
	tester.Key(ui.Cmd, ui.KeyZ)
	if a.doc.Buffer.String() != "" {
		t.Fatal("Command+Z did not undo")
	}
	tester.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if a.doc.Buffer.String() != "one two one" {
		t.Fatal("Command+Shift+Z did not redo")
	}
	tester.Key(ui.Cmd|ui.Alt, ui.KeyF)
	if !a.findOpen || !a.replacing {
		t.Fatal("Command+Option+F did not open Replace")
	}
	tester.Key(0, ui.KeyEscape)
	first := a.documentTab
	tester.Key(ui.Cmd, ui.KeyN)
	if a.documentTab == first || len(a.tabs) != 2 {
		t.Fatal("Command+N did not create a tab")
	}
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	if a.documentTab != first {
		t.Fatal("Control+Shift+Tab did not switch tabs")
	}
	if shortcutText("Ctrl+H") != "Cmd+Option+F" || shortcutText("Ctrl+Y") != "Cmd+Shift+Z" {
		t.Fatal("incorrect Mac shortcut hints")
	}
}
