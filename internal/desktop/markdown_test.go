package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const markdownExample = "# Preview heading\n\n" +
	"Native **bold** and *italic* text, ~~removed~~ and `inline code`.\n\n" +
	"> A block quote.\n\n" +
	"- [x] Ready\n- [ ] Pending\n\n" +
	"| Feature | Status |\n| --- | --- |\n| Preview | Enabled |\n\n" +
	"```go\nfunc main() {\n    println(\"Hello\")\n}\n```\n\n" +
	"[Documentation](https://example.com)\n"

func markdownApp(text string) *App {
	a := testApp()
	a.doc.Path = "example.md"
	a.doc.Buffer.Reset(text)
	a.resetEditor()
	return a
}

func TestMarkdownPreviewToggleKeepsEditsSelectionAndScroll(t *testing.T) {
	a := markdownApp(markdownExample + strings.Repeat("\nparagraph\n", 80))
	tester := ui.NewTester(a.view, 1120, 700)
	if _, ok := tester.Find(locales.MarkdownPreviewArea()); !ok || !tester.HasText("Preview heading") {
		t.Fatal("Markdown did not open with a rendered preview")
	}
	if !tester.HasText("Enabled") || !tester.HasText("A block quote.") {
		t.Fatal("Markdown table or block quote was not rendered")
	}
	a.doc.Buffer.Edit(editor.Edit{Text: "draft\n\n"})
	a.state.Select(3, 5)
	a.state.ScrollTo(30)
	tester.Frame()
	before, selection, scroll := a.doc.Buffer.Snapshot(), a.state.Selection(), a.state.ScrollY()
	if err := tester.Click(locales.MarkdownPreview()); err != nil {
		t.Fatal(err)
	}
	if _, ok := tester.Find(locales.MarkdownPreviewArea()); ok || a.preview.open || a.preview.source != "" || a.preview.options != nil {
		t.Fatal("closing the preview left its pane or cache active")
	}
	if a.doc.Buffer.Snapshot() != before || a.state.Selection() != selection || !a.doc.Buffer.CanUndo() {
		t.Fatal("closing the preview changed editor state")
	}
	if a.state.ScrollY() < scroll/2 {
		t.Fatalf("closing the preview reset the editor scroll: %v -> %v", scroll, a.state.ScrollY())
	}
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyV)
	if !a.preview.open || a.doc.Buffer.Snapshot() != before || a.state.Selection() != selection {
		t.Fatal("preview shortcut failed or changed the document")
	}
	if _, ok := tester.Find(locales.MarkdownPreviewArea()); !ok {
		t.Fatal("preview shortcut did not restore the right pane")
	}
}

func TestMarkdownPreviewRefreshDebouncesAndTracksUndoRedo(t *testing.T) {
	a := markdownApp("# Original\n")
	tester := ui.NewTester(a.view, 900, 600)
	a.doc.Buffer.Edit(editor.Edit{Start: 2, End: 10, Text: "Changed"})
	tester.Frame()
	if a.preview.source != "# Original\n" {
		t.Fatal("preview refreshed before the editing delay")
	}
	tester.Advance(markdownPreviewDelay / 2)
	a.doc.Buffer.Edit(editor.Edit{Start: 2, End: 9, Text: "Newest"})
	tester.Frame()
	tester.Advance(markdownPreviewDelay - time.Millisecond)
	if a.preview.source != "# Original\n" {
		t.Fatal("a second edit did not restart the refresh delay")
	}
	tester.Advance(2 * time.Millisecond)
	if a.preview.source != "# Newest\n" || !tester.HasText("Newest") {
		t.Fatalf("preview did not render the latest buffer: %q", a.preview.source)
	}
	a.state.Run(editor.Undo)
	tester.Frame()
	tester.Advance(markdownPreviewDelay)
	if a.preview.source != "# Changed\n" {
		t.Fatal("undo did not update the preview")
	}
	a.state.Run(editor.Redo)
	tester.Frame()
	tester.Advance(markdownPreviewDelay)
	if a.preview.source != "# Newest\n" {
		t.Fatal("redo did not update the preview")
	}
}

func TestMarkdownPreviewTabsAndLanguageChoicesAreIndependent(t *testing.T) {
	a := markdownApp("# First tab")
	tester := ui.NewTester(a.view, 760, 600)
	first := a.documentTab
	a.toggleMarkdownPreview()
	a.addDocument(document.New())
	a.doc.Path = "second.markdown"
	a.doc.Buffer.Reset("# Second tab")
	a.resetEditor()
	second := a.documentTab
	tester.Frame()
	if !a.preview.open || !tester.HasText("Second tab") || tester.HasText("First tab") {
		t.Fatal("second tab did not show its own preview")
	}
	a.preview.split = 60
	a.activateTab(first)
	tester.Frame()
	if a.preview.open || a.preview.source != "" {
		t.Fatal("tab switch lost the first tab's closed state")
	}
	a.activateTab(second)
	tester.Frame()
	if !a.preview.open || a.preview.split != 60 {
		t.Fatal("tab switch lost the preview or divider position")
	}
	a.setLanguage(languagePlain)
	tester.Frame()
	if _, ok := tester.Find(locales.MarkdownPreviewArea()); ok || a.preview.source != "" {
		t.Fatal("plain text kept a Markdown preview")
	}
	a.setLanguage("Markdown")
	tester.Frame()
	if !a.preview.open || !tester.HasText("Second tab") {
		t.Fatal("manual Markdown format did not restore the preview")
	}
	a.newDocument()
	tester.Frame()
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyV)
	if _, ok := tester.Find(locales.MarkdownPreviewArea()); ok {
		t.Fatal("preview shortcut opened a pane for a non-Markdown document")
	}
}

func TestMarkdownPreviewBoundsKeepUnicodeAndReportTruncation(t *testing.T) {
	for _, text := range []string{
		"# Small\n\nA **complete** document.\n",
		"# Heading\n\n" + strings.Repeat("中文😀", markdownPreviewBytes),
		strings.Repeat("line\n", markdownPreviewLines+100),
		"|a|b|\n|-|-|\n" + strings.Repeat("|x|y|\n", 800),
		"# Heading\n\n" + strings.Repeat("*em* ", 3000),
	} {
		snapshot := editor.NewBuffer(text).Snapshot()
		shown, truncated := markdownPreviewText(snapshot)
		if len(shown) > markdownPreviewBytes || !utf8.ValidString(shown) || !strings.HasPrefix(text, shown) {
			t.Fatal("preview bounds broke its prefix or Unicode")
		}
		if shown == "" {
			t.Fatal("preview limits hid the entire document")
		}
		if truncated != (shown != text) {
			t.Fatal("truncated content was not reported")
		}
		if strings.HasPrefix(text, "# Small") && (truncated || shown != text) {
			t.Fatal("ordinary Markdown was truncated")
		}
	}
	a := markdownApp(strings.Repeat("line\n", markdownPreviewLines+100))
	tester := ui.NewTester(a.view, 760, 600)
	if !a.preview.truncated || !tester.HasText(locales.MarkdownPreviewTruncated()) {
		t.Fatal("bounded preview omitted its truncation notice")
	}
}

func TestMarkdownPreviewClosedDoesNoTextWork(t *testing.T) {
	a := markdownApp(strings.Repeat("large file line\n", 1<<16))
	a.preview.open = false
	tester := ui.NewTester(a.view, 760, 600)
	for range 3 {
		a.doc.Buffer.Edit(editor.Edit{Text: "new\n"})
		tester.Frame()
		if a.preview.buffer != nil || a.preview.source != "" || a.preview.options != nil {
			t.Fatal("closed preview read or cached the document")
		}
	}
}

func TestMarkdownPreviewLocalImagesAndLimits(t *testing.T) {
	directory := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 12, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "image with spaces.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	load := previewFileImages(directory)
	for _, name := range []string{"image%20with%20spaces.png", path} {
		result, err := load(name)
		if err != nil || result == nil {
			t.Fatalf("local image %q failed: %v", name, err)
		}
	}
	for _, name := range []string{"https://example.com/image.png", "data:image/png;base64,abc", "//server/share/image.png", "file://server/share/image.png"} {
		if _, err := load(name); err == nil {
			t.Fatalf("non-local image was accepted: %q", name)
		}
	}
	load = previewFileImages(directory)
	var wg sync.WaitGroup
	for range previewImageCount {
		wg.Go(func() {
			if _, err := load(path); err != nil {
				t.Errorf("bounded concurrent image load failed: %v", err)
			}
		})
	}
	wg.Wait()
	if _, err := load(path); err == nil {
		t.Fatal("image count limit was ignored")
	}
}

func TestMarkdownPreviewLayoutsAndTranslations(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en"} {
		t.Setenv("MYGO_LOCALE", locale)
		for _, theme := range []string{"浅色", "深色"} {
			for _, width := range []int{760, 1120} {
				a := markdownApp(markdownExample)
				a.settings.Theme = theme
				tester := ui.NewTester(a.view, width, 420)
				tester.SetLocale(locale)
				for _, scale := range []float32{1, 1.5, 2} {
					tester.SetScale(scale)
					left, ok := tester.Find(locales.EditorArea())
					right, previewOK := tester.Find(locales.MarkdownPreviewArea())
					toggle, toggleOK := tester.Find(locales.MarkdownPreview())
					if !ok || !previewOK || !toggleOK || left.W < 200 || right.W < 200 ||
						left.X+left.W > right.X || right.X+right.W > float32(width)+0.5 || toggle.X+toggle.W > float32(width) {
						t.Fatalf("%s %s %d %v: invalid split layout: editor=%+v preview=%+v toggle=%+v",
							locale, theme, width, scale, left, right, toggle)
					}
				}
			}
		}
	}
}

func TestMarkdownPreviewImageByteAndPixelBudgets(t *testing.T) {
	directory := t.TempDir()
	big := filepath.Join(directory, "too-large.png")
	file, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(previewImageBytes + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	load := previewFileImages(directory)
	if _, err := load(big); err == nil {
		t.Fatal("oversized image input was accepted")
	}
	path := filepath.Join(directory, "tile.svg")
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="2048" height="2048"><rect width="2048" height="2048"/></svg>`), 0600); err != nil {
		t.Fatal(err)
	}
	load = previewFileImages(directory)
	for range 2 {
		if _, err := load(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := load(path); err == nil {
		t.Fatal("aggregate image pixel budget was ignored")
	}
}

func TestMarkdownPreviewImagesUseNewSaveDirectory(t *testing.T) {
	a := markdownApp("# Note")
	tester := ui.NewTester(a.view, 900, 600)
	options := a.preview.options
	a.doc.Path = filepath.Join(t.TempDir(), "saved.md")
	tester.Frame()
	if a.preview.options == options || a.preview.path != a.doc.Path {
		t.Fatal("Save As kept the old image directory")
	}
}

func BenchmarkMarkdownPreviewBoundedCopy(b *testing.B) {
	text := strings.Repeat("A large Markdown document.\n", 4<<20)
	snapshot := editor.NewBuffer(text).Snapshot()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = markdownPreviewText(snapshot)
	}
}

func BenchmarkMarkdownPreviewFrame(b *testing.B) {
	for _, example := range []struct {
		name, text string
		open       bool
	}{
		{"closed", strings.Repeat(markdownExample, 5), false},
		{"open", strings.Repeat(markdownExample, 5), true},
		{"large-table", "|a|b|\n|-|-|\n" + strings.Repeat("|x|y|\n", 800), true},
		{"large-paragraph", strings.Repeat("A large Markdown document.\n", 10000), true},
	} {
		b.Run(example.name, func(b *testing.B) {
			a := markdownApp(example.text)
			a.preview.open = example.open
			tester := ui.NewTester(a.view, 1120, 700)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				tester.Frame()
			}
		})
	}
}

func TestMarkdownPreviewScreenshots(t *testing.T) {
	directory := os.Getenv("TEXTVIEWER_MARKDOWN_PREVIEW_DIR")
	if directory == "" {
		t.Skip("set TEXTVIEWER_MARKDOWN_PREVIEW_DIR to render Markdown previews")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-Hans", "en"} {
		t.Setenv("MYGO_LOCALE", locale)
		for _, theme := range []string{"浅色", "深色"} {
			for _, width := range []int{760, 1120} {
				a := markdownApp(markdownExample)
				a.settings.Theme = theme
				tester := ui.NewTester(a.view, width, 700)
				tester.SetLocale(locale)
				path := filepath.Join(directory, fmt.Sprintf("%s-%s-%d.png", locale, theme, width))
				file, err := os.Create(path)
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
