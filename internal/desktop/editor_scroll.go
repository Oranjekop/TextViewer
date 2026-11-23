package desktop

import "github.com/egoist/mygo/ui"

func (tab *documentTab) editorInput(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown, ui.InputScroll, ui.InputKeyDown, ui.InputText, ui.InputCompose, ui.InputCommand, ui.InputSelect, ui.InputZoom:
		tab.preview.follow.editorInput = true
	case ui.InputPointerMove:
		if tab.editorPointerPressed && ev.Button == 0 {
			tab.preview.follow.editorInput = true
		}
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		tab.editorPointerPressed = ev.Button == 0
	case ui.InputPointerUp:
		tab.editorPointerPressed = false
	}
	return false
}

func (a *App) previewInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll {
		return false
	}
	dx, dy := ev.DX, ev.DY
	if ev.Mods&ui.Shift != 0 && dx == 0 {
		dx, dy = dy, 0
	}
	// Consume wheel input before MyGo starts its container scroll animation.
	s := &a.preview.scroll
	s.X = max(0, min(s.X+dx, s.MaxX))
	s.Y = max(0, min(s.Y+dy, s.MaxY))
	return true
}
