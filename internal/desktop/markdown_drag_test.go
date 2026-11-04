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

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

func previewScrollbar(t *testing.T, tester *ui.Tester, a *App) (ui.Rect, float32, float32) {
	t.Helper()
	pane, ok := tester.Find(locales.MarkdownPreviewArea())
	if !ok || a.preview.scroll.MaxY <= 0 {
		t.Fatal("preview has no scrollbar")
	}
	bar := ui.Scrollbar{Box: pane, View: pane.H, Content: pane.H + a.preview.scroll.MaxY, Offset: a.preview.scroll.Y}
	start, length := bar.Thumb()
	return pane, start, length
}

func TestMarkdownPreviewUsesAvailableHeight(t *testing.T) {
	for _, size := range [][2]int{{1120, 700}, {760, 420}} {
		a := markdownApp(markdownScrollExample())
		var left, right ui.Rect
		tester := ui.NewTester(func(c *ui.Context) {
			a.view(c)
			left = c.Resolve(a.editor).Bounds()
			right = c.Resolve(a.preview.scroll.Handle).Bounds()
		}, size[0], size[1])
		for _, findOpen := range []bool{false, true} {
			a.findOpen = findOpen
			tester.Frame()
			if left.H <= 0 || right.H <= 0 || left.Y+left.H > float32(size[1]-30)+0.25 ||
				right.Y+right.H > float32(size[1]-30)+0.25 {
				t.Fatalf("split panes extend under the status bar: window=%v find=%v left=%+v right=%+v",
					size, findOpen, left, right)
			}
		}
	}
}

func TestMarkdownPreviewScrollbarTakesOverEditorAnimation(t *testing.T) {
	a := markdownApp(markdownScrollExample())
	tester := ui.NewTester(a.view, 1120, 700)
	a.state.ScrollTo(80)
	tester.Frame()
	left, _ := tester.Find(locales.EditorArea())
	tester.Wheel(left.X+90, left.Y+60, 0, 240)
	tester.Advance(40 * time.Millisecond)
	if !a.wheel.active {
		t.Fatal("fixture did not start an editor animation")
	}
	pane, start, length := previewScrollbar(t, tester, a)
	x, y := pane.X+pane.W-5, start+length/2
	tester.Move(x, y)
	tester.Press(x, y)
	if a.wheel.active {
		t.Error("pressing the preview scrollbar did not stop the editor animation")
	}
	tester.Move(x, y+40)
	tester.Advance(40 * time.Millisecond)
	if a.wheel.active {
		t.Error("editor animation continued while dragging the preview scrollbar")
	}
	tester.Move(x, pane.Y+pane.H-2)
	tester.Release(x, pane.Y+pane.H-2)
	tester.Advance(time.Second)
	if math.Abs(float64(a.preview.scroll.Y-a.preview.scroll.MaxY)) > 0.25 {
		t.Fatalf("preview scrollbar could not reach its end: %v/%v", a.preview.scroll.Y, a.preview.scroll.MaxY)
	}
	if tester.FramePending() {
		t.Fatal("scrollbar handoff did not settle")
	}
}

func TestMarkdownPreviewScrollbarBottomFollowsEditor(t *testing.T) {
	a := markdownApp(markdownScrollExample())
	tester := ui.NewTester(a.view, 1120, 700)
	pane, start, length := previewScrollbar(t, tester, a)
	x, y := pane.X+pane.W-5, start+length/2
	tester.Move(x, y)
	tester.Press(x, y)
	tester.Move(x, pane.Y+pane.H-2)
	tester.Release(x, pane.Y+pane.H-2)
	tester.Advance(time.Second)
	if math.Abs(float64(a.preview.scroll.Y-a.preview.scroll.MaxY)) > 0.25 {
		t.Fatalf("preview drag missed its end: %v/%v", a.preview.scroll.Y, a.preview.scroll.MaxY)
	}
	left, limit := a.state.ScrollY(), markdownEditorLimit(a)
	if math.Abs(left-limit) > 0.25 {
		t.Fatalf("editor did not follow the preview's bottom: %v/%v", left, limit)
	}
}

func markdownEditorLimit(a *App) float64 {
	left := a.state.ScrollY()
	a.state.SetScrollY(math.MaxFloat64)
	limit := a.state.ScrollY()
	a.state.SetScrollY(left)
	return limit
}

func TestMarkdownScrollbarsReachBothEnds(t *testing.T) {
	examples := []struct {
		name, source string
		wrap         bool
	}{
		{"mixed", markdownScrollExample(), true},
		{"no-final-newline", strings.TrimRight(markdownScrollExample(), "\n"), true},
		{"last-wrapped-paragraph", markdownScrollExample() + strings.Repeat("A long final paragraph 中文。 ", 200), true},
		{"horizontal-scrollbar", markdownScrollExample() + "\n```\n" + strings.Repeat(strings.Repeat("wide code ", 100)+"\n", 35) + "```\n", false},
	}
	for _, example := range examples {
		for _, previewLeads := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preview=%t", example.name, previewLeads), func(t *testing.T) {
				a := markdownApp(example.source)
				a.wrap = example.wrap
				tester := ui.NewTester(a.view, 760, 600)
				tester.SetScale(1.5)
				if a.preview.truncated {
					t.Fatal("fixture should show its entire Markdown source")
				}
				before, selections := a.doc.Buffer.Snapshot(), a.state.Selections()
				var pane ui.Rect
				var start, length float32
				if previewLeads {
					pane, start, length = previewScrollbar(t, tester, a)
				} else {
					pane, _ = tester.Find(locales.EditorArea())
					bar := ui.Scrollbar{Box: pane, View: pane.H, Content: pane.H + float32(markdownEditorLimit(a))}
					start, length = bar.Thumb()
				}
				x := pane.X + pane.W - 5
				tester.Move(x, start+length/2)
				tester.Press(x, start+length/2)
				for _, fraction := range []float32{0.25, 0.7, 1, 0.5, 0} {
					y := pane.Y + 2 + (pane.H-4-length)*fraction + length/2
					tester.Move(x, y)
					tester.Advance(30 * time.Millisecond)
					left, right := a.state.ScrollY(), float64(a.preview.scroll.Y)
					leftMax, rightMax := markdownEditorLimit(a), float64(a.preview.scroll.MaxY)
					if fraction == 0 || fraction == 1 {
						if math.Abs(left-float64(fraction)*leftMax) > 0.5 ||
							math.Abs(right-float64(fraction)*rightMax) > 0.5 {
							t.Fatalf("drag endpoints did not follow: fraction=%v left=%v/%v right=%v/%v",
								fraction, left, leftMax, right, rightMax)
						}
					} else if left <= 0 || left >= leftMax || right <= 0 || right >= rightMax {
						t.Fatalf("drag lost an intermediate position: left=%v/%v right=%v/%v", left, leftMax, right, rightMax)
					}
				}
				tester.Release(x, pane.Y+2+length/2)
				tester.Advance(time.Second)
				if tester.FramePending() {
					t.Fatal("dragging kept requesting frames after release")
				}
				if a.doc.Buffer.Snapshot() != before || a.doc.Buffer.CanUndo() {
					t.Fatal("dragging changed the document or undo history")
				}
				after := a.state.Selections()
				if len(after) != len(selections) {
					t.Fatal("dragging changed the number of selections")
				}
				for i := range selections {
					if selections[i] != after[i] {
						t.Fatal("dragging changed an editor selection")
					}
				}
			})
		}
	}
}

func TestMarkdownPreviewDragSurvivesRelayout(t *testing.T) {
	a := markdownApp(markdownScrollExample())
	tester := ui.NewTester(a.view, 1120, 700)
	pane, start, length := previewScrollbar(t, tester, a)
	x, y := pane.X+pane.W-5, start+length/2
	tester.Move(x, y)
	tester.Press(x, y)
	tester.Move(x, pane.Y+pane.H/2)
	tester.SetSize(760, 600)
	a.zoom(6)
	tester.SetScale(2)
	tester.Frame()
	pane, _, _ = previewScrollbar(t, tester, a)
	x, y = pane.X+pane.W-5, pane.Y+pane.H-2
	tester.Move(x, y)
	tester.Release(x, y)
	tester.Advance(time.Second)
	if math.Abs(float64(a.preview.scroll.Y-a.preview.scroll.MaxY)) > 0.5 ||
		math.Abs(a.state.ScrollY()-markdownEditorLimit(a)) > 0.5 {
		t.Fatal("resizing or changing font/DPI broke the captured preview drag")
	}
	if tester.FramePending() {
		t.Fatal("relayout did not settle after releasing the scrollbar")
	}
}

func TestMarkdownPreviewTruncatedDragKeepsItsSourceBoundary(t *testing.T) {
	a := markdownApp(strings.Repeat("# Heading\n\nParagraph.\n\n", 2000))
	a.wrap = false
	tester := ui.NewTester(a.view, 900, 600)
	pane, start, length := previewScrollbar(t, tester, a)
	x, y := pane.X+pane.W-5, start+length/2
	tester.Move(x, y)
	tester.Press(x, y)
	tester.Move(x, pane.Y+pane.H-2)
	tester.Release(x, pane.Y+pane.H-2)
	tester.Advance(time.Second)
	first, _ := a.state.VisibleLines()
	if !a.preview.truncated || first < markdownPreviewLines-3 || first > markdownPreviewLines+1 {
		t.Fatalf("truncated preview did not follow its own source boundary: line %d", first)
	}
	a.state.SetScrollY(math.MaxFloat64)
	tester.Frame()
	left := a.state.ScrollY()
	tester.Advance(time.Second)
	if math.Abs(a.state.ScrollY()-left) > 0.25 || a.preview.scroll.Y != a.preview.scroll.MaxY {
		t.Fatal("truncated preview pulled the editor back from the full document's end")
	}
}

func TestMarkdownPreviewShortDocumentStartsAtTop(t *testing.T) {
	for _, source := range []string{"", "# Short\n", strings.Repeat("long ", 500)} {
		a := markdownApp(source)
		tester := ui.NewTester(a.view, 900, 600)
		if a.state.ScrollY() != 0 || a.preview.scroll.Y != 0 || tester.FramePending() {
			t.Fatal("initial preview jumped to the end or kept requesting frames")
		}
	}
}

func TestMarkdownPreviewDraggedToBottomScreenshot(t *testing.T) {
	directory := os.Getenv("TEXTVIEWER_MARKDOWN_PREVIEW_DIR")
	if directory == "" {
		t.Skip("set TEXTVIEWER_MARKDOWN_PREVIEW_DIR to render the final drag position")
	}
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	a := markdownApp(markdownScrollExample() + "\n# End of document\n\nFinal preview paragraph.\n")
	a.settings.Theme = "浅色"
	tester := ui.NewTester(a.view, 760, 420)
	tester.SetLocale("zh-Hans")
	pane, start, length := previewScrollbar(t, tester, a)
	x, y := pane.X+pane.W-5, start+length/2
	tester.Move(x, y)
	tester.Press(x, y)
	tester.Move(x, pane.Y+pane.H-2)
	tester.Release(x, pane.Y+pane.H-2)
	tester.Advance(time.Second)
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "drag-bottom-zh-Hans-760.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
	final, ok := tester.Find("Final preview paragraph.")
	if !ok || final.H <= 0 || final.Y < pane.Y || final.Y+final.H > pane.Y+pane.H {
		t.Fatalf("the final preview paragraph remains outside the viewport: %+v", final)
	}
	if math.Abs(float64(a.preview.scroll.Y-a.preview.scroll.MaxY)) > 0.25 {
		t.Fatal("the screenshot did not reach the preview's end")
	}
}

func BenchmarkMarkdownScrollDragLargeFile(b *testing.B) {
	example := markdownScrollExample()
	a := markdownApp(strings.Repeat(example, (100<<20)/len(example)))
	a.wrap = false
	tester := ui.NewTester(a.view, 1120, 700)
	pane, _ := tester.Find(locales.MarkdownPreviewArea())
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		delta := float32(2)
		if i%100 >= 50 {
			delta = -2
		}
		tester.Scroll(pane.X+80, pane.Y+80, 0, delta)
	}
}
