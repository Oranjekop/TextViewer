package desktop

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func markdownScrollExample() string {
	var source strings.Builder
	for section := range 24 {
		fmt.Fprintf(&source, "# Section %02d\n\n", section)
		for range 1 + section%4 {
			source.WriteString("A **paragraph** with text that wraps differently in the editor and rendered preview. 中文内容。\n\n")
		}
		if section%3 == 0 {
			source.WriteString("```go\nfunc example() {\n    println(\"code\")\n}\n```\n\n")
		}
		if section%5 == 0 {
			source.WriteString("| Column | Status |\n| --- | --- |\n| Example | Ready |\n\n")
		}
	}
	return source.String()
}

func assertMarkdownHeadingAligned(t *testing.T, tester *ui.Tester, a *App, heading string) {
	t.Helper()
	pane, ok := tester.Find(locales.MarkdownPreviewArea())
	if !ok {
		t.Fatal("preview pane is missing")
	}
	rendered, ok := tester.Find(heading)
	if !ok || math.Abs(float64(rendered.Y-pane.Y)) > 2 {
		t.Fatalf("%s did not align with the editor's first line: rendered=%+v, pane=%+v, left=%v, right=%v",
			heading, rendered, pane, a.state.ScrollY(), a.preview.scroll.Y)
	}
}

func TestMarkdownScrollFollowsEditorSourcePositions(t *testing.T) {
	source := markdownScrollExample()
	a := markdownApp(source)
	tester := ui.NewTester(a.view, 1120, 700)
	for _, section := range []int{6, 14, 9, 2} {
		heading := fmt.Sprintf("Section %02d", section)
		line := a.doc.Buffer.Snapshot().LineOf(strings.Index(source, "# "+heading))
		a.state.ScrollTo(line)
		tester.Frame()
		assertMarkdownHeadingAligned(t, tester, a, heading)
	}
	left, _ := tester.Find(locales.EditorArea())
	before := a.preview.scroll.Y
	tester.Scroll(left.X+80, left.Y+80, 0, 160)
	if a.preview.scroll.Y <= before {
		t.Fatal("scrolling the editor did not move the preview")
	}
	beforeLeft, beforeRight := a.state.ScrollY(), a.preview.scroll.Y
	for range 3 {
		tester.Frame()
	}
	if math.Abs(a.state.ScrollY()-beforeLeft) > 0.25 || math.Abs(float64(a.preview.scroll.Y-beforeRight)) > 0.25 {
		t.Fatal("programmatic following fed back into the source pane")
	}
}

func TestMarkdownScrollFollowsPreviewWithoutChangingEditorState(t *testing.T) {
	source := markdownScrollExample()
	a := markdownApp(source)
	tester := ui.NewTester(a.view, 1120, 700)
	a.state.Select(3, 7)
	tester.Frame()
	before, selection := a.doc.Buffer.Snapshot(), a.state.Selection()
	for _, section := range []int{8, 16, 4} {
		heading := fmt.Sprintf("Section %02d", section)
		pane, _ := tester.Find(locales.MarkdownPreviewArea())
		tester.Scroll(pane.X+50, pane.Y+50, 0, -a.preview.scroll.Y)
		if a.state.ScrollY() > 0.25 {
			t.Fatal("scrolling the preview to the top did not return the editor to the top")
		}
		var rendered ui.Rect
		for step := range 100 {
			rect, ok := tester.Find(heading)
			if ok && rect.H > 0 && rect.Y >= pane.Y && rect.Y < pane.Y+pane.H {
				rendered = rect
				break
			}
			if step == 99 {
				t.Fatalf("heading did not enter the preview viewport: %s", heading)
			}
			tester.Scroll(pane.X+50, pane.Y+50, 0, pane.H/3)
		}
		tester.Scroll(pane.X+50, pane.Y+50, 0, rendered.Y-pane.Y)
		line := a.doc.Buffer.Snapshot().LineOf(strings.Index(source, "# "+heading))
		if math.Abs(a.state.ScrollY()-a.state.LineTop(line)) > 2 {
			t.Fatalf("scrolling preview to %s did not reveal the corresponding source line: got %v, want %v; preview=%v/%v, heading=%+v, pane=%+v, editorLeads=%v, remembered=(%v,%v)",
				heading, a.state.ScrollY(), a.state.LineTop(line), a.preview.scroll.Y, a.preview.scroll.MaxY,
				rendered, pane, a.preview.follow.editorLeads, a.preview.follow.editorY, a.preview.follow.previewY)
		}
		assertMarkdownHeadingAligned(t, tester, a, heading)
	}
	if a.doc.Buffer.Snapshot() != before || a.state.Selection() != selection || a.doc.Buffer.CanUndo() {
		t.Fatal("scroll following changed the document, selection or undo history")
	}
	if tester.FramePending() {
		t.Fatal("scroll following did not settle")
	}
}

func TestMarkdownScrollHandlesRelayoutAndRefresh(t *testing.T) {
	source := markdownScrollExample()
	a := markdownApp(source)
	tester := ui.NewTester(a.view, 1120, 700)
	heading := "Section 12"
	line := a.doc.Buffer.Snapshot().LineOf(strings.Index(source, "# "+heading))
	a.state.ScrollTo(line)
	tester.Frame()
	for _, scale := range []float32{1, 1.5, 2} {
		tester.SetScale(scale)
		a.preview.split = 60
		a.zoom(2)
		tester.SetSize(760, 600)
		a.state.ScrollTo(line)
		tester.Frame()
		assertMarkdownHeadingAligned(t, tester, a, heading)
		if tester.FramePending() {
			t.Fatal("layout changes caused continuous scroll following")
		}
	}
	a.doc.Buffer.Edit(editor.Edit{Text: "Intro\n\n"})
	tester.Frame()
	tester.Advance(markdownPreviewDelay + time.Millisecond)
	snapshot := a.doc.Buffer.Snapshot()
	line = snapshot.LineOf(strings.Index(snapshot.String(), "# "+heading))
	a.state.ScrollTo(line)
	tester.Frame()
	assertMarkdownHeadingAligned(t, tester, a, heading)
}

func TestMarkdownScrollMouseWheelCanChangeSides(t *testing.T) {
	a := markdownApp(markdownScrollExample())
	tester := ui.NewTester(a.view, 1120, 700)
	a.state.ScrollTo(80)
	tester.Frame()
	left, _ := tester.Find(locales.EditorArea())
	right, _ := tester.Find(locales.MarkdownPreviewArea())
	for _, movement := range []struct {
		pane  ui.Rect
		delta float32
	}{{left, 150}, {right, -100}, {right, 100}, {left, -150}} {
		beforeLeft, beforeRight := a.state.ScrollY(), a.preview.scroll.Y
		x, y := movement.pane.X+100, movement.pane.Y+100
		tester.Move(x, y)
		tester.Wheel(x, y, 0, movement.delta)
		tester.Advance(time.Second)
		sign := float64(movement.delta)
		if (a.state.ScrollY()-beforeLeft)*sign <= 0 || float64(a.preview.scroll.Y-beforeRight)*sign <= 0 {
			t.Fatalf("wheel direction did not carry to both panes: left %v -> %v, right %v -> %v, delta %v",
				beforeLeft, a.state.ScrollY(), beforeRight, a.preview.scroll.Y, movement.delta)
		}
		if tester.FramePending() {
			t.Fatal("wheel scrolling did not settle")
		}
	}
}

func TestMarkdownScrollDoesNotPullEditorBackFromTruncatedContent(t *testing.T) {
	a := markdownApp(strings.Repeat("# Heading\n\nParagraph.\n\n", 2000))
	tester := ui.NewTester(a.view, 900, 600)
	if !a.preview.truncated {
		t.Fatal("fixture should exceed the preview limit")
	}
	a.state.ScrollTo(4000)
	tester.Frame()
	left := a.state.ScrollY()
	if a.preview.scroll.Y != a.preview.scroll.MaxY {
		t.Fatal("scrolling past the preview prefix did not stop at the preview's end")
	}
	for range 3 {
		tester.Frame()
		if math.Abs(a.state.ScrollY()-left) > 0.25 {
			t.Fatal("truncated preview pulled the editor back into its prefix")
		}
	}
	pane, _ := tester.Find(locales.MarkdownPreviewArea())
	tester.Scroll(pane.X+50, pane.Y+50, 0, -200)
	first, _ := a.state.VisibleLines()
	if first >= markdownPreviewLines {
		t.Fatalf("scrolling the preview did not return to its source prefix: line %d", first)
	}
}

func TestMarkdownScrollReopensAndSwitchesTabs(t *testing.T) {
	source := markdownScrollExample()
	a := markdownApp(source)
	tester := ui.NewTester(a.view, 1120, 700)
	line := a.doc.Buffer.Snapshot().LineOf(strings.Index(source, "# Section 10"))
	a.state.ScrollTo(line)
	tester.Frame()
	first := a.documentTab
	a.toggleMarkdownPreview()
	a.state.ScrollTo(line + 20)
	tester.Frame()
	if a.preview.follow.ready || len(a.preview.follow.anchors) != 0 {
		t.Fatal("closed preview retained its position map")
	}
	a.state.ScrollTo(line)
	a.toggleMarkdownPreview()
	tester.Frame()
	assertMarkdownHeadingAligned(t, tester, a, "Section 10")
	a.addDocument(document.New())
	a.doc.Path = "second.md"
	a.doc.Buffer.Reset("# Short\n")
	a.resetEditor()
	tester.Frame()
	if a.preview.scroll.Y != 0 || a.state.ScrollY() != 0 {
		t.Fatal("new tab inherited another document's scroll")
	}
	a.activateTab(first)
	tester.Frame()
	assertMarkdownHeadingAligned(t, tester, a, "Section 10")
}

var markdownScrollBenchmarkY float64

func BenchmarkMarkdownScrollMappingLargeFile(b *testing.B) {
	example := markdownScrollExample()
	text := strings.Repeat(example, (100<<20)/len(example))
	a := markdownApp(text)
	a.wrap = false
	_ = ui.NewTester(a.view, 1120, 700)
	snapshot := a.doc.Buffer.Snapshot()
	left := a.state.LineTop(100)
	right := float64(a.preview.scroll.MaxY) / 2
	b.Run("editor-to-preview", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			markdownScrollBenchmarkY = a.preview.follow.fromEditor(a.state, snapshot, left)
		}
	})
	b.Run("preview-to-editor", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			markdownScrollBenchmarkY = a.preview.follow.toEditor(a.state, snapshot, right)
		}
	})
}

func TestMarkdownScrollScreenshot(t *testing.T) {
	directory := os.Getenv("TEXTVIEWER_MARKDOWN_PREVIEW_DIR")
	if directory == "" {
		t.Skip("set TEXTVIEWER_MARKDOWN_PREVIEW_DIR to render a synchronized preview")
	}
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	source := markdownScrollExample()
	a := markdownApp(source)
	a.settings.Theme = "浅色"
	tester := ui.NewTester(a.view, 760, 700)
	tester.SetLocale("zh-Hans")
	a.state.ScrollTo(a.doc.Buffer.Snapshot().LineOf(strings.Index(source, "# Section 12")))
	tester.Frame()
	assertMarkdownHeadingAligned(t, tester, a, "Section 12")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "sync-zh-Hans-760.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
}
