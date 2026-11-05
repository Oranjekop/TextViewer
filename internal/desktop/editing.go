package desktop

import (
	"context"
	"runtime"
	"slices"

	"github.com/Oranjekop/TextViewer/internal/textops"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (a *App) commandItem(m *ui.Menu, label string, command editor.Command, mods ui.Modifiers, key ui.Key) {
	if runtime.GOOS == "darwin" {
		switch command {
		case editor.Redo:
			mods, key = ui.Cmd|ui.Shift, ui.KeyZ
		case editor.GoToLine, editor.JoinLines:
			mods = ui.Ctrl
		case editor.ToggleBlockComment:
			// MyGo's native binding is a chord, which Menu.Shortcut cannot display.
			key = 0
		case editor.SelectLarger, editor.SelectSmaller:
			mods = ui.Ctrl | ui.Shift
		}
	}
	disabled := a.busy != nil || a.confirming
	switch command {
	case editor.Undo:
		disabled = disabled || !a.doc.Buffer.CanUndo()
	case editor.Redo:
		disabled = disabled || !a.doc.Buffer.CanRedo()
	case editor.ToggleComment:
		language := a.doc.Buffer.Language()
		disabled = disabled || language == nil || language.LineComment == "" && language.BlockComment[0] == ""
	case editor.ToggleBlockComment:
		language := a.doc.Buffer.Language()
		disabled = disabled || language == nil || language.BlockComment[0] == "" || language.BlockComment[1] == ""
	}
	item := m.Item(label).Disabled(disabled)
	if key != 0 {
		item.Shortcut(mods, key)
	}
	if item.Chosen() {
		a.runEditorCommand(command)
	}
}

func (a *App) runEditorCommand(command editor.Command) {
	if a.busy != nil || a.confirming || a.fontDialog || a.settingsDialog {
		return
	}
	// Large selected case conversions should not block the UI. With a caret
	// alone, keep MyGo's word-at-caret behavior and its multi-cursor semantics.
	var op textops.Operation
	switch command {
	case editor.Uppercase:
		op = textops.Uppercase
	case editor.Lowercase:
		op = textops.Lowercase
	case editor.TitleCase:
		op = textops.TitleCase
	case editor.SnakeCase:
		op = textops.SnakeCase
	case editor.KebabCase:
		op = textops.KebabCase
	case editor.CamelCase:
		op = textops.CamelCase
	case editor.PascalCase:
		op = textops.PascalCase
	}
	if op >= textops.Uppercase {
		selections := a.state.Selections()
		if !slices.ContainsFunc(selections, func(s editor.Selection) bool { return s.Empty() }) {
			a.transformText(op)
			return
		}
	}
	before := a.doc.Buffer.Snapshot()
	a.state.Run(command)
	if a.doc.Buffer.Snapshot() != before {
		a.clearSearchResult()
		a.searchStatus, a.status = nil, nil
	}
	a.pendingFocus = command != editor.GoToLine
}

func (a *App) editingMenu(m *ui.Menu) {
	a.commandItem(m, locales.Undo(), editor.Undo, ui.Cmd, ui.KeyZ)
	a.commandItem(m, locales.Redo(), editor.Redo, ui.Cmd, ui.KeyY)
	m.Separator()
	a.commandItem(m, locales.Cut(), editor.Cut, ui.Cmd, ui.KeyX)
	a.commandItem(m, locales.Copy(), editor.Copy, ui.Cmd, ui.KeyC)
	a.commandItem(m, locales.Paste(), editor.Paste, ui.Cmd, ui.KeyV)
	a.commandItem(m, locales.Delete(), editor.Delete, 0, ui.KeyDelete)
	m.Separator()
	a.commandItem(m, locales.SelectAll(), editor.SelectAll, ui.Cmd, ui.KeyA)
	m.Submenu(locales.Selection(), a.selectionMenu)
	m.Submenu(locales.LineOperations(), a.lineOperationsMenu)
	m.Submenu(locales.WhitespaceOperations(), a.whitespaceMenu)
	m.Submenu(locales.Transform(), a.transformMenu)
	m.Separator()
	a.commandItem(m, locales.ToggleComment(), editor.ToggleComment, ui.Cmd, ui.KeySlash)
	a.commandItem(m, locales.ToggleBlockComment(), editor.ToggleBlockComment, ui.Cmd|ui.Shift, ui.KeyA)
	m.Separator()
	if m.Item(locales.Find()).Shortcut(ui.Cmd, ui.KeyF).Chosen() {
		a.showFind(false)
	}
	if m.Item(locales.Replace()).Shortcut(findReplaceShortcut()).Chosen() {
		a.showFind(true)
	}
	a.commandItem(m, locales.GoToLine(), editor.GoToLine, ui.Cmd, ui.KeyG)
	m.Separator()
	a.jsonFormatMenuItem(m)
	if m.Item(locales.CompareText()).Disabled(a.busy != nil || a.confirming || len(a.tabs) < 2).Chosen() {
		a.chooseComparison()
	}
}

func (a *App) selectionMenu(m *ui.Menu) {
	a.commandItem(m, locales.SelectLine(), editor.SelectLine, ui.Cmd, ui.KeyL)
	a.commandItem(m, locales.SelectNextOccurrence(), editor.SelectNext, ui.Cmd, ui.KeyD)
	a.commandItem(m, locales.SelectAllOccurrences(), editor.SelectAllMatches, ui.Cmd|ui.Shift, ui.KeyL)
	m.Separator()
	a.commandItem(m, locales.ExpandSelection(), editor.SelectLarger, ui.Alt|ui.Shift, ui.KeyRight)
	a.commandItem(m, locales.ShrinkSelection(), editor.SelectSmaller, ui.Alt|ui.Shift, ui.KeyLeft)
	a.commandItem(m, locales.AddCursorAbove(), editor.AddCursorAbove, ui.Cmd|ui.Alt, ui.KeyUp)
	a.commandItem(m, locales.AddCursorBelow(), editor.AddCursorBelow, ui.Cmd|ui.Alt, ui.KeyDown)
	a.commandItem(m, locales.SplitIntoLines(), editor.SplitSelectionIntoLines, 0, 0)
}

func (a *App) lineOperationsMenu(m *ui.Menu) {
	a.commandItem(m, locales.DuplicateLine(), editor.DuplicateLinesDown, ui.Alt|ui.Shift, ui.KeyDown)
	a.commandItem(m, locales.DeleteLine(), editor.DeleteLine, ui.Cmd|ui.Shift, ui.KeyK)
	a.commandItem(m, locales.MoveLineUp(), editor.MoveLinesUp, ui.Alt, ui.KeyUp)
	a.commandItem(m, locales.MoveLineDown(), editor.MoveLinesDown, ui.Alt, ui.KeyDown)
	m.Separator()
	a.commandItem(m, locales.InsertLineAbove(), editor.NewlineAbove, ui.Cmd|ui.Shift, ui.KeyEnter)
	a.commandItem(m, locales.InsertLineBelow(), editor.NewlineBelow, ui.Cmd, ui.KeyEnter)
	a.commandItem(m, locales.JoinLines(), editor.JoinLines, ui.Cmd, ui.KeyJ)
	a.commandItem(m, locales.Indent(), editor.Indent, ui.Cmd, ui.KeyBracketRight)
	a.commandItem(m, locales.Outdent(), editor.Outdent, ui.Cmd, ui.KeyBracketLeft)
	m.Separator()
	for _, item := range []struct {
		label string
		op    textops.Operation
	}{
		{locales.SortAscending(), textops.SortAscending},
		{locales.SortDescending(), textops.SortDescending},
		{locales.SortIgnoreCase(), textops.SortIgnoreCase},
		{locales.ReverseLines(), textops.ReverseLines},
		{locales.UniqueLines(), textops.UniqueLines},
	} {
		a.transformItem(m, item.label, item.op)
	}
}

func (a *App) whitespaceMenu(m *ui.Menu) {
	for _, item := range []struct {
		label string
		op    textops.Operation
	}{
		{locales.TrimLeading(), textops.TrimLeading},
		{locales.TrimTrailing(), textops.TrimTrailing},
		{locales.TrimBoth(), textops.TrimBoth},
		{locales.RemoveEmptyLines(), textops.RemoveEmptyLines},
		{locales.RemoveBlankLines(), textops.RemoveBlankLines},
		{"", 0},
		{locales.TabsToSpaces(), textops.TabsToSpaces},
		{locales.LeadingSpacesToTabs(), textops.LeadingSpacesToTabs},
	} {
		if item.label == "" {
			m.Separator()
		} else {
			a.transformItem(m, item.label, item.op)
		}
	}
}

func (a *App) transformMenu(m *ui.Menu) {
	for _, item := range []struct {
		label string
		cmd   editor.Command
	}{
		{locales.UpperCase(), editor.Uppercase},
		{locales.LowerCase(), editor.Lowercase},
		{locales.TitleCase(), editor.TitleCase},
		{locales.SnakeCase(), editor.SnakeCase},
		{locales.KebabCase(), editor.KebabCase},
		{locales.CamelCase(), editor.CamelCase},
		{locales.PascalCase(), editor.PascalCase},
	} {
		a.commandItem(m, item.label, item.cmd, 0, 0)
	}
	hasSelection := slices.ContainsFunc(a.state.Selections(), func(s editor.Selection) bool { return !s.Empty() })
	if m.Item(locales.InvertCase()).Disabled(a.busy != nil || !hasSelection).Chosen() {
		a.transformText(textops.InvertCase)
	}
	m.Separator()
	a.transformItem(m, locales.Base64Encode(), textops.Base64Encode)
	a.transformItem(m, locales.Base64Decode(), textops.Base64Decode)
}

func (a *App) transformItem(m *ui.Menu, label string, op textops.Operation) {
	if m.Item(label).Disabled(a.busy != nil || a.confirming).Chosen() {
		a.transformText(op)
	}
}

func (a *App) transformText(op textops.Operation) {
	if a.busy != nil || a.confirming {
		return
	}
	tab := a.documentTab
	buffer, snapshot := tab.doc.Buffer, tab.doc.Buffer.Snapshot()
	selections := a.state.Selections()
	tabSize := a.state.TabSize
	if language := buffer.Language(); tabSize == 0 && language != nil {
		tabSize = language.TabSize
	}
	ctx, id := a.startJob(locales.EditingText)
	a.background(func() {
		result, err := textops.Prepare(ctx, snapshot, selections, op, tabSize)
		a.updateUI(func() {
			if a.closed || a.job != id {
				return
			}
			if err == nil {
				err = ctx.Err()
			}
			finish := func() {
				a.finishJob(id, func() {
					if err != nil {
						a.fail(err)
						return
					}
					a.clearSearchResult()
					a.searchStatus, a.status = nil, nil
					if len(result.Edits) > 0 {
						tab.state.SetSelections(result.Selections...)
					}
					a.pendingFocus = true
				})
			}
			if err != nil || len(result.Edits) == 0 {
				finish()
				return
			}
			a.cancellable, a.busy = false, locales.ApplyingEdits
			a.background(func() {
				err = applyTextEdits(ctx, buffer, snapshot, result.Edits)
				finish()
			})
		})
	})
}

func applyTextEdits(ctx context.Context, buffer *editor.Buffer, snapshot editor.Snapshot, edits []editor.Edit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if buffer.Snapshot() != snapshot {
		return locales.NewError(locales.EditTextChanged)
	}
	buffer.Edit(edits...)
	return nil
}
