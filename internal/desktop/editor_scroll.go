package desktop

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const editorWheelDuration = 150 * time.Millisecond

type editorWheel struct {
	active       bool
	pressed      bool
	reduceMotion bool
	from, to     float64
	position     float64
	started      time.Time
	version      uint64
	selection    editor.Selection
}

func (tab *documentTab) editorInput(ev ui.InputEvent) bool {
	s := &tab.wheel
	switch ev.Kind {
	case ui.InputPointerDown, ui.InputScroll, ui.InputKeyDown, ui.InputText, ui.InputCompose, ui.InputCommand, ui.InputSelect, ui.InputZoom:
		tab.preview.follow.editorInput = true
	case ui.InputPointerMove:
		if s.pressed && ev.Button == 0 {
			tab.preview.follow.editorInput = true
		}
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		s.pressed = ev.Button == 0
	case ui.InputPointerUp:
		s.pressed = false
	case ui.InputPointerMove:
		return false
	case ui.InputScroll:
		if !s.pressed && !s.reduceMotion && !ev.Precise && !ev.Momentum && ev.DX == 0 && ev.DY != 0 && ev.Mods == 0 {
			state := tab.state
			from, target := state.ScrollY(), state.ScrollY()
			// Accumulate notches in one direction; reversing starts from the
			// visible position rather than waiting for pending travel.
			if s.active && math.Abs(from-s.position) < 0.01 && (s.to-from)*float64(ev.DY) > 0 {
				target = s.to
			}
			// Let the editor clamp to its actual layout, including soft wraps
			// and the space reserved for its horizontal scrollbar.
			state.SetScrollY(target + float64(ev.DY))
			target = state.ScrollY()
			state.SetScrollY(from)
			if target != from {
				s.from, s.to, s.position = from, target, from
				s.started, s.active = ev.Time, true
				s.version, s.selection = state.Buffer.Version(), state.Selection()
				return true
			}
		}
	case ui.InputKeyDown, ui.InputText, ui.InputCompose, ui.InputCommand, ui.InputSelect, ui.InputZoom:
	default:
		return false
	}
	s.active = false
	return false
}

func (s *editorWheel) frame(c *ui.Context, state *editor.State) {
	s.reduceMotion = c.Preferences().ReduceMotion
	if !s.active {
		return
	}
	if math.Abs(state.ScrollY()-s.position) >= 0.01 ||
		state.Buffer.Version() != s.version || state.Selection() != s.selection {
		s.active = false
		return
	}
	progress := min(1, max(0, float32(c.Now().Sub(s.started))/float32(editorWheelDuration)))
	if s.reduceMotion {
		progress = 1
	}
	target := s.from + (s.to-s.from)*float64(ui.EaseOut(progress))
	state.SetScrollY(target)
	s.position = state.ScrollY()
	s.active = progress < 1 && math.Abs(s.position-target) < 0.01
	if s.active {
		c.AnimationFrame()
	}
}

func (a *App) previewInput(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputScroll || ev.Kind == ui.InputPointerDown {
		a.wheel.active = false
	}
	return false
}
