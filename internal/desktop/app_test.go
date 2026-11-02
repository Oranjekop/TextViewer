package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func testApp() *App {
	a := &App{fontSize: "16", wrap: true, settings: preferences{FontSize: 16, Wrap: true, Theme: "浅色"}, integration: &fakeIntegration{}, dialogs: &fakeDialogs{}}
	a.addDocument(document.New())
	return a
}

func TestEditorTypingUndoRedo(t *testing.T) {
	a := testApp()
	tester := ui.NewTester(a.view, 1100, 720)
	tester.Type("Hello 中文😀")
	if a.doc.Buffer.String() != "Hello 中文😀" {
		t.Fatalf("got %q", a.doc.Buffer.String())
	}
	tester.Key(ui.Ctrl, ui.KeyZ)
	if a.doc.Buffer.String() != "" {
		t.Fatalf("undo: %q", a.doc.Buffer.String())
	}
	tester.Key(ui.Ctrl, ui.KeyY)
	if a.doc.Buffer.String() != "Hello 中文😀" {
		t.Fatalf("redo: %q", a.doc.Buffer.String())
	}
}

func TestCompactLayoutNoOverlap(t *testing.T) {
	for _, width := range []int{760, 900, 1120} {
		a := testApp()
		a.findOpen, a.replacing = true, true
		tester := ui.NewTester(a.view, width, 600)
		for _, row := range [][]string{
			{"文件", "打开", "保存", "另存为", "查找", "替换", "自动换行", "字体", "字号", "代码格式", "设置"},
			{"查找内容", "区分大小写", "上一处", "下一处", "关闭查找"},
			{"替换内容", "全部替换"},
		} {
			var previous ui.Rect
			for i, label := range row {
				r, ok := tester.Find(label)
				if !ok {
					t.Fatalf("width %d: %q absent", width, label)
				}
				if r.X < 0 || r.X+r.W > float32(width)+0.5 {
					t.Fatalf("width %d: %q outside window %+v", width, label, r)
				}
				if i > 0 && previous.X+previous.W > r.X+0.5 {
					t.Fatalf("width %d: overlapping %q: previous=%+v current=%+v", width, label, previous, r)
				}
				previous = r
			}
		}
	}
}

func TestFindUsesSelectionAndKeepsWrap(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set("hello 中文\nworld")
	tester := ui.NewTester(a.view, 1000, 700)
	a.state.Select(6, 12)
	tester.Key(ui.Ctrl, ui.KeyH)
	if !a.findOpen || !a.replacing || a.query != "中文" {
		t.Fatalf("find state: %v %v %q", a.findOpen, a.replacing, a.query)
	}
	a.wrap = false
	tester.Frame()
	if a.state.SoftWrap {
		t.Fatal("word wrap remains enabled")
	}
	tester.Key(0, ui.KeyEscape)
	if a.findOpen {
		t.Fatal("escape did not close find")
	}
}

func TestReadOnlyDuringBackgroundOperation(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set("original")
	a.busy = "正在打开…"
	tester := ui.NewTester(a.view, 1000, 700)
	tester.Type("should not be inserted")
	if a.doc.Buffer.String() != "original" {
		t.Fatal("busy document allowed edits")
	}
}

func TestEditingContextMenuIncludesUndoAndFind(t *testing.T) {
	a := testApp()
	a.doc.Buffer.Set("text")
	tester := ui.NewTester(a.view, 1000, 700)
	if err := tester.RightClick("文本编辑区"); err != nil {
		t.Fatal(err)
	}
	menu := strings.Join(tester.Menu(), "|")
	for _, label := range []string{"撤销", "重做", "查找", "替换"} {
		if !strings.Contains(menu, label) {
			t.Fatalf("missing %s in %s", label, menu)
		}
	}
}

func TestReplacingLoadedDocumentStartsAtBeginning(t *testing.T) {
	a := testApp()
	tester := ui.NewTester(a.view, 1000, 700)
	a.doc = document.New()
	a.doc.Buffer.Set("first\nsecond\nlast\n")
	a.resetEditor()
	a.state.Select(0, 0)
	tester.Frame()
	if cursor := a.state.Cursor(); cursor != 0 {
		t.Fatalf("opened document at %d instead of the start", cursor)
	}
}

func TestColumnCacheUnicodeAndMovement(t *testing.T) {
	s := editor.NewBuffer("a中文😀b\nsecond").Snapshot()
	var cache columnCache
	for _, tc := range []struct{ at, want int }{{0, 1}, {1, 2}, {4, 3}, {11, 5}, {12, 6}, {1, 2}, {13, 1}} {
		if got := cache.at(s, tc.at); got != tc.want {
			t.Fatalf("at %d got %d want %d", tc.at, got, tc.want)
		}
	}
}

func TestPreview(t *testing.T) {
	output := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if output == "" {
		t.Skip("set TEXTVIEWER_PREVIEW_DIR to render native previews")
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"浅色", "深色"} {
		for _, width := range []int{760, 1120} {
			for _, mode := range []string{"text", "code"} {
				a := testApp()
				a.settings.Theme = theme
				text := "TextViewer\n\n中文文本、English text、自动换行和字号设置。\n" + strings.Repeat("这是一段用于验证自动换行的长文本。", 25) + "\n\n查找、替换与撤销 / 重做。\n"
				if mode == "code" {
					a.doc.Path = "example.go"
					text = "package main\n\nimport \"fmt\"\n\n// 代码高亮不会修改文件内容。\nfunc main() {\n\tmessage := \"Hello, TextViewer\"\n\tfor i := 0; i < 3; i++ {\n\t\tfmt.Println(message, i)\n\t}\n}\n"
				}
				a.doc.Buffer.Reset(text)
				a.resetEditor()
				a.findOpen, a.replacing = true, true
				tester := ui.NewTester(a.view, width, 700)
				filename := "light"
				if theme == "深色" {
					filename = "dark"
				}
				filename += "-" + mode + "-" + strconv.Itoa(width) + ".png"
				f, err := os.Create(filepath.Join(output, filename))
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(f, tester.Image()); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
		}
	}
}
