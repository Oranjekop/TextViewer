package desktop

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func wheelApp(t *testing.T, path string, wrap bool) (*App, *ui.Tester, ui.Rect) {
	t.Helper()
	a := testApp()
	a.wrap = wrap
	a.doc.Path = path
	a.doc.Buffer.Reset(strings.Repeat("A line of text with 中文 and **formatting**.\n", 500))
	a.resetEditor()
	tester := ui.NewTester(a.view, 1120, 700)
	pane, ok := tester.Find(locales.EditorArea())
	if !ok {
		t.Fatal("editor pane is missing")
	}
	return a, tester, pane
}

func TestEditorWheelSmoothsAllFileTypes(t *testing.T) {
	for _, path := range []string{"notes.txt", "app.log", "data.json", "main.go", "README.md", "unknown.xyz"} {
		for _, wrap := range []bool{false, true} {
			t.Run(path+map[bool]string{false: "/unwrapped", true: "/wrapped"}[wrap], func(t *testing.T) {
				a, tester, pane := wheelApp(t, path, wrap)
				before, selection := a.doc.Buffer.Snapshot(), a.state.Selection()
				tester.Wheel(pane.X+90, pane.Y+60, 0, 240)
				if a.state.ScrollY() != 0 || !tester.FramePending() {
					t.Fatal("wheel jumped immediately instead of starting an animation")
				}
				tester.Advance(40 * time.Millisecond)
				if y := a.state.ScrollY(); y <= 0 || y >= 240 {
					t.Fatalf("missing intermediate scroll position: %v", y)
				}
				tester.Advance(200 * time.Millisecond)
				if math.Abs(a.state.ScrollY()-240) > 0.01 || a.wheel.active || tester.FramePending() {
					t.Fatalf("wheel failed to settle at its destination: %v", a.state.ScrollY())
				}
				if a.doc.Buffer.Snapshot() != before || a.state.Selection() != selection || a.doc.Buffer.CanUndo() {
					t.Fatal("scrolling changed text, selection or undo history")
				}
			})
		}
	}
}

func TestEditorWheelAccumulatesAndReverses(t *testing.T) {
	a, tester, pane := wheelApp(t, "notes.txt", true)
	x, y := pane.X+90, pane.Y+60
	tester.Wheel(x, y, 0, 120)
	tester.Advance(40 * time.Millisecond)
	tester.Wheel(x, y, 0, 120)
	tester.Advance(200 * time.Millisecond)
	if math.Abs(a.state.ScrollY()-240) > 0.01 {
		t.Fatalf("successive notches lost distance: %v", a.state.ScrollY())
	}
	tester.Wheel(x, y, 0, 240)
	tester.Advance(40 * time.Millisecond)
	from := a.state.ScrollY()
	tester.Wheel(x, y, 0, -60)
	tester.Advance(20 * time.Millisecond)
	if a.state.ScrollY() >= from {
		t.Fatal("reversing the wheel continued pending forward travel")
	}
	tester.Advance(200 * time.Millisecond)
	if math.Abs(a.state.ScrollY()-(from-60)) > 0.01 || tester.FramePending() {
		t.Fatal("reversed wheel did not settle")
	}
}

func TestEditorWheelLeavesTouchpadAndReducedMotionImmediate(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a, tester, pane := wheelApp(t, "data.json", true)
		tester.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		x, y := pane.X+90, pane.Y+60
		tester.Wheel(x, y, 0, 120)
		if reduced {
			if a.state.ScrollY() != 120 || a.wheel.active {
				t.Fatal("reduced motion did not scroll immediately")
			}
		} else {
			tester.Advance(40 * time.Millisecond)
		}
		from := a.state.ScrollY()
		tester.Scroll(x, y, 0, 12)
		if math.Abs(a.state.ScrollY()-(from+12)) > 0.01 {
			t.Fatal("touchpad movement was delayed")
		}
		from = a.state.ScrollY()
		tester.Advance(time.Second)
		if a.state.ScrollY() != from || a.wheel.active || tester.FramePending() {
			t.Fatal("old wheel animation continued after touchpad input")
		}
	}
	a, tester, pane := wheelApp(t, "notes.txt", true)
	tester.Wheel(pane.X+90, pane.Y+60, 0, 120)
	tester.Advance(40 * time.Millisecond)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if a.state.ScrollY() != 120 || a.wheel.active || tester.FramePending() {
		t.Fatal("enabling reduced motion did not finish pending movement")
	}
}

func TestEditorWheelClampsAtDocumentEdges(t *testing.T) {
	a, tester, pane := wheelApp(t, "notes.txt", true)
	x, y := pane.X+90, pane.Y+60
	tester.Wheel(x, y, 0, -120)
	if a.state.ScrollY() != 0 || a.wheel.active || tester.FramePending() {
		t.Fatal("scrolling above the document started an animation")
	}
	a.state.SetScrollY(1e20)
	tester.Frame()
	end := a.state.ScrollY()
	tester.Wheel(x, y, 0, 1e9)
	if a.state.ScrollY() != end || a.wheel.active {
		t.Fatal("scrolling beyond the document started an animation")
	}
	tester.Wheel(x, y, 0, -120)
	tester.Advance(200 * time.Millisecond)
	if math.Abs(a.state.ScrollY()-(end-120)) > 0.01 {
		t.Fatal("clamped travel delayed scrolling away from the bottom")
	}
	a.doc.Buffer.Reset("Short file")
	tester.Frame()
	tester.Wheel(x, y, 0, 120)
	if a.state.ScrollY() != 0 || a.wheel.active || tester.FramePending() {
		t.Fatal("short documents kept an unnecessary animation alive")
	}
}

func TestEditorWheelYieldsToNavigationAndEditing(t *testing.T) {
	for _, action := range []struct {
		name string
		run  func(*App, *ui.Tester, ui.Rect)
	}{
		{"keyboard", func(a *App, tt *ui.Tester, _ ui.Rect) { tt.Key(ui.Ctrl, ui.KeyEnd) }},
		{"pointer", func(a *App, tt *ui.Tester, pane ui.Rect) { tt.ClickAt(pane.X+140, pane.Y+80) }},
		{"selection", func(a *App, tt *ui.Tester, _ ui.Rect) { a.state.Select(2000, 2010) }},
		{"programmatic scroll", func(a *App, tt *ui.Tester, _ ui.Rect) { a.state.ScrollTo(100) }},
		{"edit", func(a *App, tt *ui.Tester, _ ui.Rect) { a.doc.Buffer.Edit(editor.Edit{Text: "Intro\n"}) }},
	} {
		t.Run(action.name, func(t *testing.T) {
			a, tester, pane := wheelApp(t, "notes.txt", true)
			tester.Wheel(pane.X+90, pane.Y+60, 0, 240)
			tester.Advance(40 * time.Millisecond)
			action.run(a, tester, pane)
			tester.Frame()
			from := a.state.ScrollY()
			tester.Advance(time.Second)
			if a.state.ScrollY() != from || a.wheel.active || tester.FramePending() {
				t.Fatal("wheel animation fought navigation or editing")
			}
		})
	}
}

func TestEditorWheelStopsWhenSwitchingTabs(t *testing.T) {
	a, tester, pane := wheelApp(t, "notes.txt", true)
	tester.Wheel(pane.X+90, pane.Y+60, 0, 240)
	tester.Advance(40 * time.Millisecond)
	first, position := a.documentTab, a.state.ScrollY()
	a.addDocument(document.New())
	tester.Frame()
	tester.Advance(time.Second)
	if a.state.ScrollY() != 0 {
		t.Fatal("a new tab inherited pending movement")
	}
	a.activateTab(first)
	tester.Frame()
	tester.Advance(time.Second)
	if a.state.ScrollY() != position || a.wheel.active || tester.FramePending() {
		t.Fatal("an inactive tab resumed an old wheel animation")
	}
}

func TestEditorWheelPreservesMultipleSelections(t *testing.T) {
	a, tester, pane := wheelApp(t, "main.go", true)
	a.state.SetSelections(editor.Selection{Anchor: 0, Head: 5}, editor.Selection{Anchor: 55, Head: 60})
	tester.Frame()
	before := a.state.Selections()
	tester.Wheel(pane.X+90, pane.Y+60, 0, 120)
	tester.Advance(200 * time.Millisecond)
	after := a.state.Selections()
	if len(before) != len(after) {
		t.Fatal("scrolling changed the number of selections")
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatal("scrolling changed a secondary selection")
		}
	}
}

func TestEditorWheelSettlesAfterLayoutChanges(t *testing.T) {
	for _, change := range []struct {
		name string
		run  func(*App, *ui.Tester)
	}{
		{"font size", func(a *App, tt *ui.Tester) { a.zoom(6) }},
		{"window size", func(a *App, tt *ui.Tester) { tt.SetSize(760, 420) }},
		{"display scale", func(a *App, tt *ui.Tester) { tt.SetScale(2) }},
		{"word wrap", func(a *App, tt *ui.Tester) { a.wrap = false }},
	} {
		t.Run(change.name, func(t *testing.T) {
			a, tester, pane := wheelApp(t, "notes.txt", true)
			a.state.ScrollTo(100)
			tester.Frame()
			tester.Wheel(pane.X+90, pane.Y+60, 0, 120)
			tester.Advance(40 * time.Millisecond)
			change.run(a, tester)
			tester.Frame()
			tester.Advance(time.Second)
			position := a.state.ScrollY()
			tester.Advance(time.Second)
			if a.state.ScrollY() != position || a.wheel.active || tester.FramePending() {
				t.Fatal("layout changes kept a wheel animation alive")
			}
		})
	}
}

func TestEditorWheelHandsMarkdownScrollingToPreview(t *testing.T) {
	a := markdownApp(markdownScrollExample())
	tester := ui.NewTester(a.view, 1120, 700)
	a.state.ScrollTo(80)
	tester.Frame()
	left, _ := tester.Find(locales.EditorArea())
	right, _ := tester.Find(locales.MarkdownPreviewArea())
	before := a.preview.scroll.Y
	tester.Wheel(left.X+90, left.Y+60, 0, 240)
	tester.Advance(40 * time.Millisecond)
	if a.preview.scroll.Y <= before || !a.wheel.active {
		t.Fatal("preview did not follow intermediate editor movement")
	}
	before = a.preview.scroll.Y
	tester.Wheel(right.X+90, right.Y+60, 0, -120)
	tester.Advance(40 * time.Millisecond)
	if a.wheel.active || a.preview.scroll.Y >= before {
		t.Fatal("pending editor movement took over scrolling the preview")
	}
	tester.Advance(time.Second)
	if tester.FramePending() {
		t.Fatal("Markdown following kept requesting frames after scrolling")
	}
}

func BenchmarkEditorWheelInputLargeFile(b *testing.B) {
	a := testApp()
	a.wrap = false
	line := "A line of text with 中文 and formatting.\n"
	a.doc.Buffer.Reset(strings.Repeat(line, (100<<20)/len(line)))
	a.resetEditor()
	tester := ui.NewTester(a.view, 1120, 700)
	a.state.ScrollTo(100)
	tester.Frame()
	ev := ui.InputEvent{Kind: ui.InputScroll, DY: 120, Time: tester.Now()}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.wheel.active = false
		a.editorInput(ev)
	}
}

func BenchmarkEditorWheelFrameLargeFile(b *testing.B) {
	a := testApp()
	a.wrap = false
	line := "A line of text with 中文 and formatting.\n"
	a.doc.Buffer.Reset(strings.Repeat(line, (100<<20)/len(line)))
	a.resetEditor()
	tester := ui.NewTester(a.view, 1120, 700)
	pane, _ := tester.Find(locales.EditorArea())
	a.state.ScrollTo(100)
	tester.Frame()
	position := a.state.ScrollY()
	b.Run("idle", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			tester.Frame()
		}
	})
	b.Run("moving", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			b.StopTimer()
			a.wheel.active = false
			a.state.SetScrollY(position)
			tester.Frame()
			tester.Wheel(pane.X+90, pane.Y+60, 0, 120)
			b.StartTimer()
			tester.Advance(time.Second / 60)
		}
	})
}
