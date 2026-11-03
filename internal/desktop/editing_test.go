package desktop

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/internal/textops"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func chooseEditingItem(t *testing.T, tester *ui.Tester, path ...string) {
	t.Helper()
	if err := tester.Click(locales.Edit()); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem(path...); err != nil {
		t.Fatal(err)
	}
}

func TestEditingMenuClipboardUndoAndFocus(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("alpha\nbeta")
	tester := ui.NewTester(a.view, 760, 600)
	a.state.Select(0, 5)
	chooseEditingItem(t, tester, locales.Copy())
	if tester.Clipboard() != "alpha" || a.doc.Buffer.String() != "alpha\nbeta" {
		t.Fatal("copy did not use the editor selection")
	}
	chooseEditingItem(t, tester, locales.Cut())
	if a.doc.Buffer.String() != "\nbeta" || tester.Clipboard() != "alpha" {
		t.Fatal("cut did not use the editor selection")
	}
	chooseEditingItem(t, tester, locales.Undo())
	if a.doc.Buffer.String() != "alpha\nbeta" {
		t.Fatal("menu undo did not restore the cut")
	}
	chooseEditingItem(t, tester, locales.Redo())
	if a.doc.Buffer.String() != "\nbeta" {
		t.Fatal("menu redo did not repeat the cut")
	}
	tester.SetClipboard("中文😀")
	a.state.Select(0, 0)
	chooseEditingItem(t, tester, locales.Paste())
	if a.doc.Buffer.String() != "中文😀\nbeta" {
		t.Fatal("menu paste did not use the clipboard")
	}
	tester.Type("!")
	if !strings.HasPrefix(a.doc.Buffer.String(), "中文😀!") || !tester.Focused(locales.EditorArea()) {
		t.Fatal("editing menu did not return keyboard focus to the editor")
	}
}

func TestEditingMenuNativeLineCommands(t *testing.T) {
	const original = "one\n中文😀\nlast"
	for _, tc := range []struct {
		label, want string
	}{
		{locales.DuplicateLine(), "one\n中文😀\n中文😀\nlast"},
		{locales.DeleteLine(), "one\nlast"},
		{locales.MoveLineUp(), "中文😀\none\nlast"},
		{locales.MoveLineDown(), "one\nlast\n中文😀"},
		{locales.InsertLineAbove(), "one\n\n中文😀\nlast"},
		{locales.InsertLineBelow(), "one\n中文😀\n\nlast"},
		{locales.JoinLines(), "one\n中文😀 last"},
		{locales.Indent(), "one\n    中文😀\nlast"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			a := testApp()
			a.doc.Buffer.Reset(original)
			a.doc.Encoding = document.UTF16BE
			a.doc.Buffer.SetLineEnding("\r\n")
			tester := ui.NewTester(a.view, 760, 600)
			a.state.Select(4, 4)
			chooseEditingItem(t, tester, locales.LineOperations(), tc.label)
			if got := a.doc.Buffer.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if a.doc.Encoding != document.UTF16BE || a.doc.Buffer.LineEnding() != "\r\n" {
				t.Fatal("line operation changed the save format")
			}
			chooseEditingItem(t, tester, locales.Undo())
			if a.doc.Buffer.String() != original || a.doc.Buffer.CanUndo() {
				t.Fatal("line operation was not a single undo step")
			}
		})
	}
}

func TestEditingMenuBatchScopeAndConversion(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("keep \nb \na \nlast \n")
	a.doc.Encoding = document.GB18030
	a.doc.Buffer.SetLineEnding("\r\n")
	tester := ui.NewTester(a.view, 760, 600)
	a.state.Select(6, 12)
	chooseEditingItem(t, tester, locales.WhitespaceOperations(), locales.TrimTrailing())
	if got := a.doc.Buffer.String(); got != "keep \nb\na\nlast \n" {
		t.Fatalf("trim affected unselected lines: %q", got)
	}
	a.state.Select(6, 10)
	chooseEditingItem(t, tester, locales.LineOperations(), locales.SortAscending())
	if got := a.doc.Buffer.String(); got != "keep \na\nb\nlast \n" {
		t.Fatalf("sort affected unselected lines: %q", got)
	}
	a.state.Select(6, 9)
	chooseEditingItem(t, tester, locales.Transform(), locales.UpperCase())
	if got := a.doc.Buffer.String(); got != "keep \nA\nB\nlast \n" {
		t.Fatalf("case conversion escaped selection: %q", got)
	}
	if a.doc.Encoding != document.GB18030 || a.doc.Buffer.LineEnding() != "\r\n" || a.busy != nil {
		t.Fatal("batch edit did not finish with the original save format")
	}
	chooseEditingItem(t, tester, locales.Undo())
	if a.doc.Buffer.String() != "keep \na\nb\nlast \n" {
		t.Fatal("batch case conversion cannot be undone in one step")
	}
}

func TestEditingMenuWordNamingAndMultiCursor(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("helloWorld keep")
	tester := ui.NewTester(a.view, 760, 600)
	a.state.Select(3, 3)
	chooseEditingItem(t, tester, locales.Transform(), locales.SnakeCase())
	if a.doc.Buffer.String() != "hello_world keep" {
		t.Fatal("case conversion with a caret did not use the current word")
	}
	a.doc.Buffer.Reset("ab\ncd")
	a.state.SetSelections(editor.Selection{Head: 1}, editor.Selection{Head: 4})
	tester.Frame()
	chooseEditingItem(t, tester, locales.LineOperations(), locales.DuplicateLine())
	if got := a.doc.Buffer.String(); got != "ab\ncd\nab\ncd" {
		t.Fatalf("multi-cursor line operation: %q", got)
	}
}

func TestEditingMenuSelectionCommentsAndGoTo(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "en")
	a := testApp()
	a.doc.Path = "example.go"
	a.doc.Buffer.Reset("first\nsecond\nthird")
	a.resetEditor()
	tester := ui.NewTester(a.view, 1120, 700)
	tester.SetLocale("en")
	a.state.Select(0, 0)
	chooseEditingItem(t, tester, locales.ToggleComment())
	if !strings.HasPrefix(a.doc.Buffer.String(), "// first") {
		t.Fatal("line comment command did not use the code language")
	}
	chooseEditingItem(t, tester, locales.Undo())
	chooseEditingItem(t, tester, locales.GoToLine())
	tester.Key(ui.Ctrl, ui.KeyA)
	tester.Type("3")
	tester.Key(0, ui.KeyEnter)
	if a.state.CursorPos().Line != 2 {
		t.Fatalf("Go to Line did not move to line 3: %+v", a.state.CursorPos())
	}
	chooseEditingItem(t, tester, locales.Selection(), locales.SelectLine())
	if a.state.SelectedText() != "third" {
		t.Fatalf("Select Line: %q", a.state.SelectedText())
	}
}

func TestEditingMenuDisablesUnsupportedComments(t *testing.T) {
	for _, name := range []string{"example.txt", "example.py"} {
		a := testApp()
		a.doc.Path = name
		a.doc.Buffer.Reset("text")
		a.resetEditor()
		tester := ui.NewTester(a.view, 760, 600)
		if err := tester.Click(locales.Edit()); err != nil {
			t.Fatal(err)
		}
		if err := tester.ChooseMenuItem(locales.ToggleBlockComment()); err == nil {
			t.Fatalf("%s allowed an unsupported block comment", name)
		}
		if a.doc.Buffer.String() != "text" {
			t.Fatal("unsupported comment modified the document")
		}
	}
}

func TestEditingGuardsAndStalePreparedEdits(t *testing.T) {
	for _, change := range []string{"cancel", "text", "line-ending"} {
		t.Run(change, func(t *testing.T) {
			buffer := editor.NewBuffer("b\na\n")
			snapshot := buffer.Snapshot()
			result, err := textops.Prepare(context.Background(), snapshot, nil, textops.SortAscending, 4)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch change {
			case "cancel":
				cancel()
			case "text":
				buffer.Set("external change")
			case "line-ending":
				buffer.SetLineEnding("\r\n")
			}
			before := buffer.Snapshot()
			if err := applyTextEdits(ctx, buffer, snapshot, result.Edits); err == nil || buffer.Snapshot() != before {
				t.Fatal("prepared edit overwrote a cancellation or stale snapshot")
			}
		})
	}
	a := testApp()
	a.doc.Buffer.Reset("keep")
	a.busy = locales.EditingText
	before := a.doc.Buffer.Snapshot()
	a.runEditorCommand(editor.DeleteLine)
	a.transformText(textops.RemoveBlankLines)
	if a.doc.Buffer.Snapshot() != before {
		t.Fatal("busy editor accepted a second operation")
	}
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click(locales.Edit()); err != nil {
		t.Fatal(err)
	}
	if len(tester.Menu()) != 0 {
		t.Fatal("busy edit menu opened")
	}
}

func TestBackgroundEditingUsesItsOriginalSnapshot(t *testing.T) {
	buffer := editor.NewBuffer(strings.Repeat("b\na\n", 10000))
	snapshot := buffer.Snapshot()
	type outcome struct {
		result textops.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := textops.Prepare(context.Background(), snapshot, nil, textops.UniqueLines, 4)
		done <- outcome{result, err}
	}()
	buffer.Set("newer contents")
	prepared := <-done
	if prepared.err != nil || len(prepared.result.Edits) != 1 || prepared.result.Edits[0].Text != "b\na\n" {
		t.Fatalf("background result did not use its immutable snapshot: %+v", prepared)
	}
	if err := applyTextEdits(context.Background(), buffer, snapshot, prepared.result.Edits); err == nil || buffer.String() != "newer contents" {
		t.Fatal("background result overwrote a newer document")
	}
}

func TestEditingPreservesOtherTabsAndNoOpHistory(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("first\n")
	first := a.documentTab
	before := first.doc.Buffer.Snapshot()
	a.addDocument(document.New())
	a.doc.Buffer.Reset("b\na\n")
	tester := ui.NewTester(a.view, 760, 600)
	chooseEditingItem(t, tester, locales.LineOperations(), locales.SortAscending())
	if a.doc.Buffer.String() != "a\nb\n" || first.doc.Buffer.Snapshot() != before {
		t.Fatal("batch edit modified another tab")
	}
	chooseEditingItem(t, tester, locales.Undo())
	if a.doc.Buffer.String() != "b\na\n" || a.doc.Buffer.CanUndo() {
		t.Fatal("batch sort did not produce exactly one undo step")
	}
	chooseEditingItem(t, tester, locales.WhitespaceOperations(), locales.TrimTrailing())
	if a.doc.Buffer.CanUndo() {
		t.Fatal("unchanged whitespace operation created undo history")
	}
}

func TestEditingMenuDoesNotOverrideFindInputShortcuts(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("document")
	a.findOpen, a.query = true, "query"
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click(locales.QueryLabel()); err != nil {
		t.Fatal(err)
	}
	tester.Key(ui.Ctrl, ui.KeyA)
	tester.Key(ui.Ctrl, ui.KeyX)
	if a.query != "" || tester.Clipboard() != "query" || a.doc.Buffer.String() != "document" {
		t.Fatal("editor commands took the find input's shortcut")
	}
}

func TestEditingMenuTranslationsLayoutAndPreview(t *testing.T) {
	output := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if output != "" {
		if err := os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tag := range []string{"zh-Hans", "en"} {
		t.Run(tag, func(t *testing.T) {
			t.Setenv("MYGO_LOCALE", tag)
			for _, theme := range []string{"浅色", "深色"} {
				a := testApp()
				a.settings.Theme = theme
				a.doc.Buffer.Reset("alpha\n中文😀\n beta  \n")
				tester := ui.NewTester(a.view, 760, 420)
				tester.SetLocale(tag)
				for _, scale := range []float32{1, 1.5, 2} {
					tester.SetScale(scale)
					if err := tester.Click(locales.Edit()); err != nil {
						t.Fatal(err)
					}
					for _, label := range []string{locales.Cut(), locales.Copy(), locales.Paste(), locales.LineOperations(), locales.WhitespaceOperations(), locales.Transform(), locales.Selection(), locales.GoToLine()} {
						if !slices.Contains(tester.Menu(), label) {
							t.Fatalf("%s missing from %v", label, tester.Menu())
						}
					}
					tester.CloseMenu()
					var previous ui.Rect
					for i, label := range []string{locales.File(), locales.Open(), locales.Save(), locales.SaveAs(), locales.Edit(), locales.Find(), locales.Replace(), locales.WordWrap(), locales.Font(), locales.FontSize(), locales.CodeFormat(), locales.Settings()} {
						rect, ok := tester.Find(label)
						if !ok || rect.W <= 0 || rect.X < 0 || rect.X+rect.W > 760 || i > 0 && previous.X+previous.W > rect.X+0.5 {
							t.Fatalf("%s scale %g: clipped or overlapping %q: %+v", tag, scale, label, rect)
						}
						previous = rect
					}
					writeLocalePreview(t, output, fmt.Sprintf("editing-%s-%s-%g", tag, theme, scale), tester)
				}
				chooseEditingItem(t, tester, locales.LineOperations(), locales.SortDescending())
				if a.doc.Buffer.String() != "中文😀\nalpha\n beta  \n" {
					t.Fatal("translated submenu did not execute the selected action")
				}
			}
		})
	}
}

func BenchmarkEditingMenuLargeDocument(b *testing.B) {
	a := testApp()
	a.wrap = false
	a.doc.Buffer.Reset(strings.Repeat("a line of text\n", (100<<20)/15))
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click(locales.Edit()); err != nil {
		b.Fatal(err)
	}
	tester.CloseMenu()
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		if err := tester.Click(locales.Edit()); err != nil {
			b.Fatal(err)
		}
		tester.CloseMenu()
	}
}
