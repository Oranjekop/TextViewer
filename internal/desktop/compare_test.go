package desktop

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestComparisonMenuAndSourcePreservation(t *testing.T) {
	a := testApp()
	left := a.documentTab
	left.doc.Buffer.Reset("same\nbefore\nlast")
	left.doc.Buffer.Edit(editor.Edit{Start: 0, Text: "first\n"})
	left.state.Select(2, 4)
	left.doc.Encoding = document.GB18030
	left.doc.Buffer.SetLineEnding("\r\n")
	a.addDocument(document.New())
	right := a.documentTab
	right.doc.Buffer.Reset("first\nsame\nafter\nlast\nadded")
	right.state.Select(5, 2)
	leftSnapshot, rightSnapshot := left.doc.Buffer.Snapshot(), right.doc.Buffer.Snapshot()
	leftSelection, rightSelection := left.state.Selections(), right.state.Selections()
	tester := ui.NewTester(a.view, 760, 420)
	chooseEditingItem(t, tester, locales.CompareText())
	if !a.compareDialog || a.compareLeft != right || a.compareRight != left {
		t.Fatal("comparison selector did not use active and next tabs")
	}
	if err := tester.Click(locales.StartComparison()); err != nil {
		t.Fatal(err)
	}
	if a.comparison == nil || a.compareDialog {
		t.Fatal("comparison did not open")
	}
	if a.comparison.hunks == 0 || a.comparison.diff.Editable || a.comparison.diff.HunkActions {
		t.Fatal("expected read-only differences")
	}
	if a.comparison.diff.Old == right.doc.Buffer || a.comparison.diff.New == left.doc.Buffer {
		t.Fatal("comparison uses live source buffers")
	}
	for _, label := range []string{locales.NextDifference(), locales.PreviousDifference()} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	tester.Key(ui.Ctrl, ui.KeyZ)
	tester.Type("do not edit")
	if err := tester.Click(locales.CloseComparison()); err != nil {
		t.Fatal(err)
	}
	if a.comparison != nil || a.documentTab != right || len(a.tabs) != 2 {
		t.Fatal("closing comparison did not restore the active tab")
	}
	if left.doc.Buffer.Snapshot() != leftSnapshot || right.doc.Buffer.Snapshot() != rightSnapshot ||
		!slices.Equal(leftSelection, left.state.Selections()) || !slices.Equal(rightSelection, right.state.Selections()) ||
		left.doc.Encoding != document.GB18030 || left.doc.Buffer.LineEnding() != "\r\n" ||
		!left.doc.Buffer.CanUndo() || right.doc.Buffer.CanUndo() {
		t.Fatal("comparison changed source text, selection, save format or history")
	}
}

func TestComparisonLimitsCancellationAndIdenticalText(t *testing.T) {
	empty := editor.NewBuffer("").Snapshot()
	for _, input := range []string{
		strings.Repeat("x", maxCompareBytes+1),
		strings.Repeat("\n", maxCompareLines),
	} {
		oversized := editor.NewBuffer(input).Snapshot()
		if _, err := prepareComparison(context.Background(), oversized, empty); err == nil {
			t.Fatal("missing left input limit")
		}
		if _, err := prepareComparison(context.Background(), empty, oversized); err == nil {
			t.Fatal("missing right input limit")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareComparison(ctx, empty, empty); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	for _, text := range []string{"", "same\n中文😀", "same\n"} {
		snapshot := editor.NewBuffer(text).Snapshot()
		result, err := prepareComparison(context.Background(), snapshot, snapshot)
		if err != nil || result.hunks != 0 || result.added != 0 || result.deleted != 0 {
			t.Fatalf("identical text: %+v %v", result, err)
		}
	}
}

func TestComparisonEscapeAndInvalidSelection(t *testing.T) {
	a := testApp()
	a.chooseComparison()
	if a.compareDialog {
		t.Fatal("comparison needs two tabs")
	}
	a.addDocument(document.New())
	tester := ui.NewTester(a.view, 760, 420)
	a.chooseComparison()
	a.compareRight = a.compareLeft
	a.startComparison()
	if a.comparison != nil || a.busy != nil {
		t.Fatal("same tab must not compare")
	}
	tester.Key(0, ui.KeyEscape)
	if a.compareDialog {
		t.Fatal("escape must close selector")
	}
	if a.compareLeft != nil || a.compareRight != nil {
		t.Fatal("canceled selector must release source references")
	}
	a.chooseComparison()
	a.startComparison()
	tester.Key(0, ui.KeyEscape)
	if a.comparison != nil {
		t.Fatal("escape must close comparison")
	}
	a.chooseComparison()
	a.startComparison()
	tester.Key(ui.Ctrl, ui.KeyW)
	if a.comparison != nil || len(a.tabs) != 2 {
		t.Fatal("Ctrl+W must close comparison, not the source tab")
	}
}

func TestComparisonLayoutAndPreview(t *testing.T) {
	for _, size := range []struct{ w, h int }{{760, 420}, {1120, 780}} {
		t.Run(fmt.Sprint(size.w), func(t *testing.T) {
			a := testApp()
			a.doc.Path = `C:\left\sample.txt`
			a.doc.Buffer.Reset(strings.Repeat("shared line\n", 12) + "old value\n")
			a.addDocument(document.New())
			a.doc.Path = `C:\right\sample.txt`
			a.doc.Buffer.Reset(strings.Repeat("shared line\n", 12) + "new value\nadded\n")
			a.chooseComparison()
			a.startComparison()
			tester := ui.NewTester(a.view, size.w, size.h)
			for _, label := range []string{locales.PreviousDifference(), locales.NextDifference(), locales.CloseComparison()} {
				e, ok := tester.Find(label)
				if !ok || e.Y+e.H > float32(size.h) || e.X+e.W > float32(size.w) {
					t.Fatalf("control clipped: %s %+v", label, e)
				}
			}
			if dir := os.Getenv("TEXTVIEWER_COMPARE_PREVIEWS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("comparison-%d.png", size.w)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tester.Image())
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("screenshot: %v %v", err, closeErr)
				}
			}
		})
	}
}

func BenchmarkComparisonPrepare(b *testing.B) {
	left := editor.NewBuffer(strings.Repeat("same text\n", 20000) + "old\n").Snapshot()
	right := editor.NewBuffer(strings.Repeat("same text\n", 20000) + "new\n").Snapshot()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := prepareComparison(context.Background(), left, right); err != nil {
			b.Fatal(err)
		}
	}
}

func TestComparisonLocalizedLayouts(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en"} {
		t.Setenv("MYGO_LOCALE", locale)
		for _, theme := range []string{"浅色", "深色"} {
			a := testApp()
			a.settings.Theme = theme
			a.doc.Buffer.Reset("before")
			a.addDocument(document.New())
			a.doc.Buffer.Reset("after")
			tester := ui.NewTester(a.view, 760, 420)
			tester.SetLocale(locale)
			a.chooseComparison()
			for _, scale := range []float32{1, 1.5, 2} {
				tester.SetScale(scale)
				for _, label := range []string{locales.StartComparison(), locales.Cancel()} {
					if r, ok := tester.Find(label); !ok || r.X+r.W > 760 || r.Y+r.H > 420 {
						t.Fatalf("%s %s %v: selector control clipped: %s %+v", locale, theme, scale, label, r)
					}
				}
			}
			a.startComparison()
			for _, scale := range []float32{1, 1.5, 2} {
				tester.SetScale(scale)
				var previous ui.Rect
				for _, label := range []string{locales.PreviousDifference(), locales.NextDifference(), locales.CloseComparison()} {
					r, ok := tester.Find(label)
					if !ok || r.X < previous.X+previous.W || r.X+r.W > 760 || r.Y+r.H > 420 {
						t.Fatalf("%s %s %v: comparison control clipped: %s %+v", locale, theme, scale, label, r)
					}
					previous = r
				}
			}
		}
	}
}

func TestComparisonSnapshotsAndStats(t *testing.T) {
	for _, tc := range []struct {
		old, new       string
		added, deleted int
	}{
		{"", "new", 1, 0},
		{"old", "", 0, 1},
		{"same\nold", "same\nnew", 1, 1},
		{"same\n", "same\nadded\n", 1, 0},
	} {
		oldBuf, newBuf := editor.NewBuffer(tc.old), editor.NewBuffer(tc.new)
		result, err := prepareComparison(context.Background(), oldBuf.Snapshot(), newBuf.Snapshot())
		if err != nil || result.added != tc.added || result.deleted != tc.deleted {
			t.Fatalf("stats for %q / %q: %+v %v", tc.old, tc.new, result, err)
		}
		oldBuf.Reset("changed externally")
		newBuf.Reset("changed externally")
		if result.diff.Old.String() != tc.old || result.diff.New.String() != tc.new {
			t.Fatal("comparison must retain original snapshots")
		}
		added, deleted := result.diff.Stats()
		if added != tc.added || deleted != tc.deleted {
			t.Fatal("source edits changed snapshot differences")
		}
	}
}

func BenchmarkComparisonStableFrame(b *testing.B) {
	a := testApp()
	left := editor.NewBuffer(strings.Repeat("same text\n", 20000) + "old\n").Snapshot()
	right := editor.NewBuffer(strings.Repeat("same text\n", 20000) + "new\n").Snapshot()
	result, err := prepareComparison(context.Background(), left, right)
	if err != nil {
		b.Fatal(err)
	}
	a.comparison = result
	tester := ui.NewTester(a.view, 760, 420)
	tester.Frame()
	b.ReportAllocs()
	for b.Loop() {
		tester.Frame()
	}
}

func BenchmarkComparisonUnrelated(b *testing.B) {
	var old, new strings.Builder
	for i := range maxCompareLines - 1 {
		fmt.Fprintf(&old, "old %d\n", i)
		fmt.Fprintf(&new, "new %d\n", i)
	}
	left, right := editor.NewBuffer(old.String()).Snapshot(), editor.NewBuffer(new.String()).Snapshot()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := prepareComparison(context.Background(), left, right); err != nil {
			b.Fatal(err)
		}
	}
}
