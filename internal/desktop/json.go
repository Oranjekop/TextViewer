package desktop

import (
	"context"
	"errors"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func (a *App) jsonFormatMenuItem(m *ui.Menu) {
	if m.Item("格式化 JSON").Shortcut(ui.Ctrl|ui.Shift, ui.KeyJ).Disabled(a.busy != "").Chosen() {
		a.formatJSON()
	}
}

func (a *App) formatJSON() {
	if a.busy != "" {
		return
	}
	doc, snapshot := a.doc, a.doc.Buffer.Snapshot()
	ctx, id := a.startJob("正在格式化 JSON…")
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
						a.status = "JSON 已是格式化状态"
					}
					a.pendingFocus = true
				})
				return
			}
			// The final buffer edit also builds its chunk tree. Keep it off the
			// UI thread; cancellation ends before this single undoable commit.
			a.cancellable = false
			a.busy = "正在应用格式化…"
			go func() {
				err := applyJSONFormat(ctx, doc, snapshot, text)
				a.finishJob(id, func() {
					if err != nil {
						a.fail(err)
						return
					}
					a.state.Select(0, 0)
					a.searchStatus, a.status = "", ""
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
		return errors.New("文本或换行格式已改变，请重新执行格式化")
	}
	doc.Buffer.Set(text)
	return nil
}
