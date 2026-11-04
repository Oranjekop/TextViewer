package desktop

import (
	"math"
	"sort"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown/parser"
)

const markdownScrollTolerance = 0.25

type markdownScrollAnchor struct {
	offset int
	y      float64
}

type markdownScrollSync struct {
	anchors     []markdownScrollAnchor
	count       int
	top         float32
	valid       bool
	awaitLayout bool
	ready       bool
	changed     bool
	geometry    bool
	started     bool
	editorLeads bool
	editorInput bool
	editorY     float64
	editorLine  int
	editorOff   float64
	previewY    float64
	editorEnd   float64
	previewEnd  float64
	limit       int
}

func (s *markdownScrollSync) invalidate() {
	s.ready, s.started, s.awaitLayout = false, false, true
}

func (s *markdownScrollSync) begin(content ui.Rect) {
	s.top, s.count, s.changed = content.Y, 0, false
	s.valid = !s.awaitLayout && content.W > 0
	if s.valid {
		s.anchor(0, 0)
	}
}

// A zero-height marker leaves the framework's default Markdown rendering intact.
func (s *markdownScrollSync) block(c *ui.Context, d *parser.Document, i int) bool {
	block := d.Block(i)
	if block.Parent != 0 || block.Kind == parser.FootnoteDefinition {
		return false
	}
	marker := ui.Box(c.Key("source-position")).FillWidth().Height(0).Bounds()
	if marker.W <= 0 {
		s.valid = false
	}
	if s.valid {
		s.anchor(block.Start, float64(marker.Y-s.top))
	}
	return false
}

func (s *markdownScrollSync) anchor(offset int, y float64) {
	if s.count > 0 {
		last := s.anchors[s.count-1]
		if offset <= last.offset || y <= last.y {
			return
		}
	}
	anchor := markdownScrollAnchor{offset: offset, y: y}
	if s.count == len(s.anchors) {
		s.anchors = append(s.anchors, anchor)
		s.changed = true
	} else {
		old := s.anchors[s.count]
		s.changed = s.changed || old.offset != offset || math.Abs(old.y-y) > markdownScrollTolerance
		s.anchors[s.count] = anchor
	}
	s.count++
}

func (s *markdownScrollSync) end(c *ui.Context, size int, body ui.Rect) {
	if !s.valid || body.W <= 0 {
		s.ready, s.awaitLayout = false, false
		c.Invalidate()
		return
	}
	s.anchor(size, float64(body.Y-s.top+body.H)+20)
	s.changed = s.changed || s.count != len(s.anchors)
	s.anchors = s.anchors[:s.count]
	s.ready = true
	if s.changed {
		s.geometry = true
		c.Invalidate()
	}
}

func (s *markdownScrollSync) sync(c *ui.Context, state *editor.State, text editor.Snapshot, preview *ui.ScrollState) {
	if !s.ready || len(s.anchors) < 2 {
		return
	}
	left, right := state.ScrollY(), float64(preview.Y)
	leftShifted := math.Abs(left-s.editorY) > markdownScrollTolerance
	leftMoved := leftShifted
	if leftShifted && s.started {
		line, off := markdownEditorPlace(state, text, left)
		// Measuring wrapped lines above the viewport preserves its source
		// line and offset. That correction must not take over a preview drag.
		leftMoved = line != s.editorLine || math.Abs(off-s.editorOff) > markdownScrollTolerance
	}
	rightMoved := s.previewMoved(preview)
	if !s.started || s.editorInput || leftMoved && !rightMoved {
		s.editorLeads = true
	} else if rightMoved {
		s.editorLeads = false
	}
	if !s.started || leftShifted || rightMoved || s.geometry || s.editorInput {
		s.scrollRange(state, text, float64(preview.MaxY))
		if s.editorLeads {
			target := float32(max(0, min(s.fromEditor(state, text, left), float64(preview.MaxY))))
			if math.Abs(float64(target)-right) > markdownScrollTolerance {
				preview.Y = target
				c.Invalidate()
			}
		} else if preview.MaxY > 0 {
			target := s.toEditor(state, text, right)
			if math.Abs(target-left) > markdownScrollTolerance {
				state.SetScrollY(target)
				c.Invalidate()
			}
		}
		s.editorLine, s.editorOff = markdownEditorPlace(state, text, state.ScrollY())
	}
	// Remember the applied destination too, so it cannot become a new driver.
	s.editorY, s.previewY = state.ScrollY(), float64(preview.Y)
	s.started, s.geometry, s.editorInput = true, false, false
}

func (s *markdownScrollSync) previewMoved(preview *ui.ScrollState) bool {
	// Content shrinking can clamp the right pane without a user scroll.
	return s.started && math.Abs(float64(preview.Y)-min(s.previewY, float64(preview.MaxY))) > markdownScrollTolerance
}

func markdownEditorPlace(state *editor.State, text editor.Snapshot, y float64) (int, float64) {
	line := sort.Search(text.LineCount(), func(i int) bool { return state.LineTop(i+1) > y })
	line = min(line, text.LineCount()-1)
	return line, y - state.LineTop(line)
}

func (s *markdownScrollSync) scrollRange(state *editor.State, text editor.Snapshot, previewEnd float64) {
	// Query the editor's own limit, including wrapped last lines, folding
	// and horizontal scrollbar space, without changing its visible place.
	left := state.ScrollY()
	state.SetScrollY(math.MaxFloat64)
	editorEnd := state.ScrollY()
	state.SetScrollY(left)
	last := s.anchors[len(s.anchors)-1]
	s.editorEnd = min(editorEnd, markdownSourceY(state, text, last.offset))
	s.previewEnd = previewEnd
	// Keep content anchors that both viewports can reach. Interpolate the
	// remaining tail to their actual ends, so neither side stops early.
	s.limit = sort.Search(len(s.anchors), func(i int) bool {
		return s.anchors[i].y >= previewEnd || markdownSourceY(state, text, s.anchors[i].offset) >= s.editorEnd
	})
}

func markdownSourceY(state *editor.State, text editor.Snapshot, offset int) float64 {
	offset = max(0, min(offset, text.Len()))
	line := text.LineOf(offset)
	top := state.LineTop(line)
	start, end := text.LineStart(line), text.LineEnd(line)
	if offset <= start || end <= start {
		return top
	}
	fraction := min(1, float64(offset-start)/float64(end-start))
	return top + fraction*(state.LineTop(line+1)-top)
}

func (s *markdownScrollSync) fromEditor(state *editor.State, text editor.Snapshot, y float64) float64 {
	if s.editorEnd <= 0 || s.previewEnd <= 0 {
		return 0
	}
	if y >= s.editorEnd {
		return s.previewEnd
	}
	index := sort.Search(s.limit, func(i int) bool {
		return markdownSourceY(state, text, s.anchors[i].offset) >= y
	})
	if index == 0 {
		return 0
	}
	before := s.anchors[index-1]
	if index == s.limit {
		return markdownInterpolate(y, markdownSourceY(state, text, before.offset),
			s.editorEnd, before.y, s.previewEnd)
	}
	after := s.anchors[index]
	return markdownInterpolate(y, markdownSourceY(state, text, before.offset),
		markdownSourceY(state, text, after.offset), before.y, after.y)
}

func (s *markdownScrollSync) toEditor(state *editor.State, text editor.Snapshot, y float64) float64 {
	if s.editorEnd <= 0 || s.previewEnd <= 0 {
		return 0
	}
	if y >= s.previewEnd {
		return s.editorEnd
	}
	index := sort.Search(s.limit, func(i int) bool { return s.anchors[i].y >= y })
	if index == 0 {
		return 0
	}
	before := s.anchors[index-1]
	if index == s.limit {
		return markdownInterpolate(y, before.y, s.previewEnd,
			markdownSourceY(state, text, before.offset), s.editorEnd)
	}
	after := s.anchors[index]
	return markdownInterpolate(y, before.y, after.y, markdownSourceY(state, text, before.offset),
		markdownSourceY(state, text, after.offset))
}

func markdownInterpolate(value, start, end, from, to float64) float64 {
	if end <= start {
		return from
	}
	return from + max(0, min(1, (value-start)/(end-start)))*(to-from)
}
