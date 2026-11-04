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
	editorY     float64
	previewY    float64
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
	leftMoved := math.Abs(left-s.editorY) > markdownScrollTolerance
	// Content shrinking can clamp the right pane without a user scroll.
	rightMoved := math.Abs(right-min(s.previewY, float64(preview.MaxY))) > markdownScrollTolerance
	if !s.started || leftMoved {
		s.editorLeads = true
	} else if rightMoved {
		s.editorLeads = false
	}
	if !s.started || leftMoved || rightMoved || s.geometry {
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
	}
	// Remember the applied destination too, so it cannot become a new driver.
	s.editorY, s.previewY = state.ScrollY(), float64(preview.Y)
	s.started, s.geometry = true, false
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
	index := sort.Search(len(s.anchors), func(i int) bool {
		return markdownSourceY(state, text, s.anchors[i].offset) >= y
	})
	if index == 0 {
		return s.anchors[0].y
	}
	if index == len(s.anchors) {
		return s.anchors[index-1].y
	}
	before, after := s.anchors[index-1], s.anchors[index]
	return markdownInterpolate(y, markdownSourceY(state, text, before.offset),
		markdownSourceY(state, text, after.offset), before.y, after.y)
}

func (s *markdownScrollSync) toEditor(state *editor.State, text editor.Snapshot, y float64) float64 {
	index := sort.Search(len(s.anchors), func(i int) bool { return s.anchors[i].y >= y })
	if index == 0 {
		return markdownSourceY(state, text, s.anchors[0].offset)
	}
	if index == len(s.anchors) {
		return markdownSourceY(state, text, s.anchors[index-1].offset)
	}
	before, after := s.anchors[index-1], s.anchors[index]
	return markdownInterpolate(y, before.y, after.y, markdownSourceY(state, text, before.offset),
		markdownSourceY(state, text, after.offset))
}

func markdownInterpolate(value, start, end, from, to float64) float64 {
	if end <= start {
		return from
	}
	return from + max(0, min(1, (value-start)/(end-start)))*(to-from)
}
