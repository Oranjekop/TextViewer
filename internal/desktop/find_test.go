package desktop

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/internal/search"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestRegexFindRepeatsEmptyMatchesAndWraps(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("中文\n😀\nlast")
	a.query, a.regex = "^", true
	for _, want := range []int{0, 7, 12, 0, 7} {
		a.find(false)
		if selection := a.state.Selection(); selection.Start() != want || !selection.Empty() {
			t.Fatalf("next empty match: got %+v want %d, status=%q", selection, want, a.searchStatus)
		}
	}
	for _, want := range []int{0, 12, 7, 0} {
		a.find(true)
		if selection := a.state.Selection(); selection.Start() != want || !selection.Empty() {
			t.Fatalf("previous empty match: got %+v want %d, status=%q", selection, want, a.searchStatus)
		}
	}
}

func TestRegexFindAndReplaceOneExpandsCaptures(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("name=1\nother=22\n")
	a.query, a.replacement, a.regex = `(?P<name>\w+)=(\d+)`, `${name}:\t$2`, true
	a.find(false)
	if a.state.SelectedText() != "name=1" {
		t.Fatalf("selected %q", a.state.SelectedText())
	}
	a.replaceOne()
	if text := a.doc.Buffer.String(); text != "name:\t1\nother=22\n" {
		t.Fatalf("single replacement: %q; status=%q", text, a.searchStatus)
	}
	if a.state.SelectedText() != "other=22" {
		t.Fatal("single replacement did not select the next match")
	}
	a.state.Run(editor.Undo)
	if a.doc.Buffer.String() != "name=1\nother=22\n" {
		t.Fatal("single replacement cannot be undone")
	}
}

func TestSwitchingFromRegexFindToReplacePreservesPattern(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("name=123\nother=22")
	a.findOpen, a.regex = true, true
	a.query, a.replacement = `(\w+)=(\d+)`, "$2:$1"
	a.find(false)
	tester := ui.NewTester(a.view, 760, 600)
	tester.Key(ui.Ctrl, ui.KeyH)
	if !a.replacing || a.query != `(\w+)=(\d+)` {
		t.Fatalf("opening replace lost the original pattern: %q", a.query)
	}
	a.replaceOne()
	if a.doc.Buffer.String() != "123:name\nother=22" {
		t.Fatal("switching to replace lost capture groups")
	}
}

func TestReplaceAllRegexAndEscapesPreservesSaveFormatAndUndo(t *testing.T) {
	for _, regex := range []bool{false, true} {
		a := testApp()
		a.doc.Buffer.Reset("first\r\nsecond\r\n")
		a.doc.Encoding = document.UTF16LE
		a.query, a.replacement, a.regex, a.escapeSequences = `\n`, `\t`, regex, !regex
		before := a.doc.Buffer.Snapshot()
		a.replaceAll()
		if a.doc.Buffer.String() != "first\tsecond\t" || a.searchStatus.String() != "已替换 2 处" || a.busy != nil {
			t.Fatalf("regex=%v: text=%q status=%q busy=%q", regex, a.doc.Buffer.String(), a.searchStatus, a.busy)
		}
		if a.doc.Buffer.LineEnding() != "\r\n" || a.doc.Encoding != document.UTF16LE {
			t.Fatal("replacement changed the file's save format")
		}
		a.doc.Buffer.Undo()
		if a.doc.Buffer.String() != before.String() {
			t.Fatal("replace all was not one undoable edit")
		}
	}
}

func TestInvalidRegexIsReportedInFindBarAndPreservesDocument(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("keep this text")
	a.findOpen, a.replacing, a.regex, a.query = true, true, true, "["
	before := a.doc.Buffer.Snapshot()
	for _, run := range []func(){func() { a.find(false) }, a.replaceOne, a.replaceAll} {
		run()
		if !a.searchFailed || !strings.Contains(a.searchStatus.String(), "正则表达式无效") || a.doc.Buffer.Snapshot() != before {
			t.Fatalf("invalid regex changed text or lost inline error: %q", a.searchStatus)
		}
		if len(a.dialogs.(*fakeDialogs).messages) != 0 {
			t.Fatal("invalid search opened a blocking dialog")
		}
	}
	tester := ui.NewTester(a.view, 760, 420)
	if _, ok := tester.Find("查找结果"); !ok {
		t.Fatal("find error is not visible")
	}
	if err := tester.Click("正则"); err != nil {
		t.Fatal(err)
	}
	if a.regex || a.searchStatus != nil || a.searchFailed {
		t.Fatal("changing mode kept a stale error")
	}
}

func TestSearchOptionsAndSelectionEscaping(t *testing.T) {
	for _, regex := range []bool{false, true} {
		a := testApp()
		a.doc.Buffer.Reset("a.*\\n\nnext")
		a.regex, a.escapeSequences = regex, !regex
		a.state.Select(0, a.doc.Buffer.Len())
		a.showFind(true)
		a.state.Select(0, 0)
		a.find(false)
		if a.state.SelectedText() != a.doc.Buffer.String() {
			t.Fatalf("regex=%v selected text was interpreted as a pattern: query=%q status=%q", regex, a.query, a.searchStatus)
		}
	}
	a := testApp()
	a.findOpen = true
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("转义字符"); err != nil {
		t.Fatal(err)
	}
	if !a.escapeSequences {
		t.Fatal("escape option was not applied")
	}
	if err := tester.Click("正则"); err != nil {
		t.Fatal(err)
	}
	if !a.regex {
		t.Fatal("regex option was not applied")
	}
}

func TestFindStateDoesNotLeakBetweenTabs(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("a\nb")
	a.query, a.regex = "^", true
	a.find(false)
	first := a.documentTab
	a.newDocument()
	a.doc.Buffer.Reset("c\nd")
	a.find(false)
	if a.state.Cursor() != 0 {
		t.Fatal("new tab skipped its first empty match")
	}
	a.activateTab(first)
	a.find(false)
	if a.state.Cursor() != 2 {
		t.Fatal("tab lost its previous empty match position")
	}
}

func TestReplaceOneDoesNotTreatSelectionAsWholeDocument(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("foobar\nfoo\n")
	a.query, a.replacement, a.regex = "foo$", "X", true
	a.state.Select(0, 3)
	a.replaceOne()
	if a.doc.Buffer.String() != "foobar\nfoo\n" || a.state.SelectedText() != "foo" || a.state.Cursor() != 10 {
		t.Fatalf("partial selection was replaced or the next real match was not selected: text=%q selection=%+v", a.doc.Buffer.String(), a.state.Selection())
	}
	a.replaceOne()
	if a.doc.Buffer.String() != "foobar\nX\n" {
		t.Fatal("exact match was not replaced")
	}
}

func TestEmptyRegexReplacementAdvancesPastInsertedText(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("中文\nnext")
	a.query, a.replacement, a.regex = "^", ">", true
	a.find(false)
	a.replaceOne()
	if a.doc.Buffer.String() != ">中文\nnext" || a.state.Cursor() != 8 {
		t.Fatalf("zero-width replacement did not advance to the next line: %q cursor=%d", a.doc.Buffer.String(), a.state.Cursor())
	}
}

func TestApplySearchEditRejectsCancellationAndStaleSnapshots(t *testing.T) {
	for _, reason := range []string{"cancel", "text", "eol"} {
		buffer := editor.NewBuffer("original\ntext")
		snapshot := buffer.Snapshot()
		ctx, cancel := context.WithCancel(context.Background())
		switch reason {
		case "cancel":
			cancel()
		case "text":
			buffer.Set("new contents")
		case "eol":
			buffer.SetLineEnding("\r\n")
		}
		before := buffer.Snapshot()
		called := false
		err := applySearchEdit(ctx, buffer, snapshot, func() { called = true })
		cancel()
		if err == nil || called || buffer.Snapshot() != before {
			t.Fatalf("%s: stale/canceled edit was allowed: %v", reason, err)
		}
		if reason == "cancel" && !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error lost: %v", err)
		}
	}
}

func TestSearchBarFitsAtDifferentScalesAndShowsBoundedErrors(t *testing.T) {
	for _, theme := range []string{"浅色", "深色"} {
		for _, scale := range []float32{1, 1.5, 2} {
			a := testApp()
			a.settings.Theme = theme
			a.findOpen, a.replacing, a.regex = true, true, true
			a.query = "["
			a.searchError(fmt.Errorf("正则表达式无效：%s", strings.Repeat("very long invalid expression ", 40)))
			tester := ui.NewTester(a.view, 1120, 420)
			tester.SetScale(scale)
			for _, width := range []int{760, 1120} {
				tester.SetSize(width, 420)
				var previous ui.Rect
				for i, label := range []string{"查找内容", "区分大小写", "正则", "转义字符", "上一处", "下一处", "关闭查找"} {
					rect, ok := tester.Find(label)
					if !ok || rect.X < 0 || rect.X+rect.W > float32(width)+0.5 ||
						(i > 0 && previous.X+previous.W > rect.X+0.5) {
						t.Fatalf("%s scale=%g width=%d %s: previous=%+v rect=%+v", theme, scale, width, label, previous, rect)
					}
					previous = rect
				}
				errorRect, ok := tester.Find("查找结果")
				editorRect, editorOK := tester.Find("文本编辑区")
				if !ok || !editorOK || errorRect.X+errorRect.W > float32(width) || errorRect.H > 24 ||
					errorRect.Y+errorRect.H > editorRect.Y || editorRect.H < 150 {
					t.Fatalf("search error clipped or displaced the editor: error=%+v editor=%+v", errorRect, editorRect)
				}
			}
		}
	}
}

func TestSearchDoesNotInterpretForwardSlashAsEscape(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("/new/name\nnext")
	a.query, a.escapeSequences = "/n", true
	a.find(false)
	if a.state.SelectedText() != "/n" {
		t.Fatal("literal forward slashes were converted into control characters")
	}
}

func TestEscapedReplacementSavesChosenNewlines(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Reset("first|second")
	a.doc.Buffer.SetLineEnding("\r\n")
	a.query, a.replacement, a.escapeSequences = "|", `\n`, true
	a.replaceAll()
	path := filepath.Join(t.TempDir(), "result.txt")
	a.dialogs.(*fakeDialogs).save = func(mygo.SaveDialogOptions) (string, error) { return path, nil }
	a.save(true, nil)
	text, err := os.ReadFile(path)
	if err != nil || string(text) != "first\r\nsecond" {
		t.Fatalf("saved replacement: %q err=%v", text, err)
	}
}

func TestFindPreview(t *testing.T) {
	output := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if output == "" {
		t.Skip("set TEXTVIEWER_PREVIEW_DIR to render search previews")
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"浅色", "深色"} {
		a := testApp()
		a.settings.Theme = theme
		a.doc.Path = "example.txt"
		a.doc.Buffer.Reset("name=123\nother=22\n\n中文文本、换行和制表符。\n")
		a.findOpen, a.replacing, a.regex = true, true, true
		a.query, a.replacement = `(?P<name>\w+)=(\d+)`, `${name}:\t$2`
		a.find(false)
		tester := ui.NewTester(a.view, 1120, 420)
		for _, width := range []int{760, 1120} {
			tester.SetSize(width, 420)
			for _, scale := range []float32{1, 1.5, 2} {
				tester.SetScale(scale)
				file, err := os.Create(filepath.Join(output, fmt.Sprintf("search-%s-%d-%g.png", theme, width, scale)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tester.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func BenchmarkRegexReplacementPreparation(b *testing.B) {
	snapshot := editor.NewBuffer(strings.Repeat("key=value\n", (100<<20)/10)).Snapshot()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = search.ReplaceSelection(context.Background(), snapshot, search.Query{Text: "missing", Regex: true}, "X", search.Match{Start: 0, End: 3})
	}
}
