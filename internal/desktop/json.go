package desktop

import (
	"context"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (a *App) jsonFormatMenuItem(m *ui.Menu) {
	if m.Item(locales.FormatJSON()).Shortcut(ui.Ctrl|ui.Shift, ui.KeyJ).Disabled(a.busy != nil).Chosen() {
		a.formatJSON()
	}
}

func (a *App) formatJSON() {
	if a.busy != nil {
		return
	}
	doc, snapshot := a.doc, a.doc.Buffer.Snapshot()
	ctx, id := a.startJob(locales.FormattingJSON)
	go func() {
		text, changed, err := document.FormatJSON(ctx, snapshot)
		a.Win.Update(func() {
			if a.closed || a.job != id {
				return
			}
			if err == nil {
				err = ctx.Err()
			}
			if err != nil || !changed {
				a.finishJob(id, func() {
					if err != nil {
						a.fail(err)
					} else {
						a.status = locales.JsonAlreadyFormatted
					}
					a.pendingFocus = true
				})
				return
			}
			// The final buffer edit also builds its chunk tree. Keep it off the
			// UI thread; cancellation ends before this single undoable commit.
			a.cancellable = false
			a.busy = locales.ApplyingFormat
			go func() {
				err := applyJSONFormat(ctx, doc, snapshot, text)
				a.finishJob(id, func() {
					if err != nil {
						a.fail(err)
						return
					}
					a.state.Select(0, 0)
					a.searchStatus, a.status = nil, nil
					a.pendingFocus = true
				})
			}()
		})
	}()
}

func applyJSONFormat(ctx context.Context, doc *document.Document, snapshot editor.Snapshot, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if doc.Buffer.Snapshot() != snapshot {
		return locales.NewError(locales.FormatTextChanged)
	}
	doc.Buffer.Set(text)
	return nil
}
