package desktop

import (
	"fmt"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

const (
	markdownPreviewBytes = 64 << 10
	markdownPreviewLines = 1000
	markdownPreviewNodes = 1500
	markdownPreviewDelay = 250 * time.Millisecond
)

type markdownPreview struct {
	open      bool
	split     float32
	source    string
	truncated bool
	buffer    *editor.Buffer
	version   uint64
	seen      uint64
	due       time.Time
	options   *markdown.Options
	theme     markdown.Theme
	path      string
	scroll    ui.ScrollState
	follow    markdownScrollSync
}

func (a *App) isMarkdown() bool {
	language := a.doc.Buffer.Language()
	return language != nil && language.Name == "Markdown"
}

func (a *App) toggleMarkdownPreview() {
	if !a.isMarkdown() || a.busy != nil || a.confirming {
		return
	}
	a.preview.open = !a.preview.open
	if !a.preview.open {
		a.preview.release()
	}
	a.pendingFocus = true
}

func (p *markdownPreview) release() {
	p.source, p.buffer, p.options = "", nil, nil
	p.due = time.Time{}
	p.truncated = false
	p.follow = markdownScrollSync{}
}

func (a *App) documentView(c *ui.Context) {
	if !a.preview.follow.editorInput && a.preview.follow.previewMoved(&a.preview.scroll) {
		a.wheel.active = false
	}
	a.wheel.frame(c, a.state)
	if !a.isMarkdown() || !a.preview.open {
		a.preview.release()
		a.editorView(c)
		return
	}
	a.preview.update(c, a.doc.Buffer)
	if a.preview.version == a.doc.Buffer.Version() {
		a.preview.follow.sync(c, a.state, a.doc.Buffer.Snapshot(), &a.preview.scroll)
	}
	ui.SplitPercent(c.Key(fmt.Sprintf("markdown-split-%d", a.id)), &a.preview.split,
		func() { a.editorView(c) },
		func() { a.markdownView(c) },
	).FillWidth().Grow(1).MinHeight(0)
}

func (a *App) editorView(c *ui.Context) {
	in := editor.View(c.Key(fmt.Sprintf("document-%d", a.id)), a.state).Fill().Bind(&a.editor)
	if in.PressedOutside() {
		a.wheel.active = false
	}
	if a.pendingFocus {
		in.Focus()
		a.pendingFocus = false
	}
}

func (a *App) markdownView(c *ui.Context) {
	p := &a.preview
	if p.options == nil || p.path != a.doc.Path {
		p.path = a.doc.Path
		p.options = &markdown.Options{WrapCode: true, Block: p.follow.block}
		if p.path != "" {
			p.options.LoadImage = previewFileImages(filepath.Dir(p.path))
		}
	}
	p.theme.Font, p.theme.CodeFont, p.theme.FontSize = a.settings.FontFamily, a.settings.FontFamily, a.state.Font.Size
	p.options.Theme = &p.theme
	ui.Scroll(c.Key(fmt.Sprintf("markdown-preview-%d", a.id))).TrackScroll(&p.scroll).HandleInput(a.previewInput).
		Fill().Label(locales.MarkdownPreviewArea()).Children(func() {
		content := ui.Column(c).FillWidth().Padding(20, 24)
		p.follow.begin(content.Bounds())
		content.Children(func() {
			body := markdown.View(c.Key("content"), p.source, p.options).FillWidth()
			p.follow.end(c, len(p.source), body.Bounds())
			if p.truncated {
				ui.Text(c, locales.MarkdownPreviewTruncated()).FillWidth().
					FontSize(12).TextColor(c.Theme().TextMuted).Margin(16, 0, 0, 0)
			}
		})
	})
}

func (p *markdownPreview) update(c *ui.Context, buffer *editor.Buffer) {
	version := buffer.Version()
	if p.buffer != buffer {
		p.buffer, p.seen, p.version = buffer, version, version
		p.source, p.truncated = markdownPreviewText(buffer.Snapshot())
		p.follow.invalidate()
		p.due = time.Time{}
		return
	}
	if version != p.seen {
		p.seen, p.due = version, c.Now().Add(markdownPreviewDelay)
	}
	if version == p.version {
		return
	}
	if remaining := p.due.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	p.source, p.truncated = markdownPreviewText(buffer.Snapshot())
	p.follow.invalidate()
	p.version, p.due = version, time.Time{}
}

func markdownPreviewText(snapshot editor.Snapshot) (string, bool) {
	end := min(snapshot.Len(), markdownPreviewBytes)
	if snapshot.LineCount() > markdownPreviewLines {
		end = min(end, snapshot.LineStart(markdownPreviewLines))
	}
	source := snapshot.Slice(0, end)
	for len(source) > 0 {
		r, size := utf8.DecodeLastRuneInString(source)
		if r != utf8.RuneError || size != 1 {
			break
		}
		source = source[:len(source)-1]
	}
	// Bound rendered structure too: a small table can have many cells.
	doc := parser.Parse(source)
	budget := markdownPreviewNodes
	for index := 1; index < doc.Len(); index++ {
		block := doc.Block(index)
		if budget == 0 {
			source = source[:block.Start]
			break
		}
		budget--
		inlines := doc.Inlines(index)
		if len(inlines) > budget {
			source = source[:max(block.Start, min(inlines[budget].Start, len(source)))]
			break
		}
		budget -= len(inlines)
	}
	return source, len(source) < snapshot.Len()
}
