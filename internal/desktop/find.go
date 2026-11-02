package desktop

import (
	"context"
	"fmt"

	"github.com/Oranjekop/TextViewer/internal/search"
	"github.com/egoist/mygo/ui"
)

func (a *App) showFind(replacing bool) {
	a.findOpen, a.replacing = true, replacing
	sel := a.state.Selection()
	if size := sel.End() - sel.Start(); size > 0 && size < 4096 {
		a.query = a.state.SelectedText()
	}
	a.queryInput.Focus()
}

func (a *App) findBar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(10, 14).Gap(8).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.TextInput(c, &a.query).Grow(1).MinWidth(160).Placeholder("查找").Label("查找内容").Bind(&a.queryInput).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Key == ui.KeyEnter {
						a.find(k.Mods&ui.Shift != 0)
						return true
					}
					return false
				}).Changed() {
				a.searchStatus = ""
			}
			ui.Checkbox(c, &a.caseSensitive, "区分大小写")
			if ui.Button(c, "上一处").Disabled(a.busy != "" || a.query == "").Clicked() {
				a.find(true)
			}
			if ui.Button(c, "下一处").Disabled(a.busy != "" || a.query == "").Clicked() {
				a.find(false)
			}
			if ui.Button(c, "关闭").Label("关闭查找").Clicked() {
				a.findOpen = false
				a.editor.Focus()
			}
		})
		if a.replacing {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.replacement).Grow(1).MinWidth(160).Placeholder("替换为").Label("替换内容")
				if ui.Button(c, "替换").Disabled(a.busy != "" || a.query == "").Clicked() {
					a.replaceOne()
				}
				if ui.Button(c, "全部替换").Disabled(a.busy != "" || a.query == "").Clicked() {
					a.replaceAll()
				}
			})
		}
		if a.searchStatus != "" {
			ui.Text(c, a.searchStatus).FontSize(12).TextColor(t.TextMuted)
		}
	})
}

func (a *App) searchQuery() search.Query {
	return search.Query{Text: a.query, CaseSensitive: a.caseSensitive}
}

func (a *App) find(backward bool) {
	if a.busy != "" || a.query == "" {
		return
	}
	snapshot := a.doc.Buffer.Snapshot()
	sel := a.state.Selection()
	at := sel.End()
	if backward {
		at = sel.Start()
	}
	query := a.searchQuery()
	ctx, id := a.startJob("正在查找…")
	a.searchStatus = ""
	go func() {
		match, ok, err := search.Next(ctx, snapshot, query, at, backward)
		a.finishJob(id, func() {
			if err != nil {
				if err != context.Canceled {
					a.fail(err)
				}
				return
			}
			if !ok {
				a.searchStatus = "未找到“" + query.Text + "”"
				return
			}
			a.state.Select(match.Start, match.End)
			a.searchStatus = ""
		})
	}()
}

func (a *App) replaceOne() {
	if a.busy != "" || a.query == "" {
		return
	}
	sel := a.state.Selection()
	snapshot := a.doc.Buffer.Snapshot()
	found, exact := false, false
	_ = search.Scan(context.Background(), snapshot, a.searchQuery(), sel.Start(), sel.End(), func(m search.Match) bool {
		found = true
		exact = m.Start == sel.Start() && m.End == sel.End()
		return false
	})
	if !found || !exact {
		a.find(false)
		return
	}
	a.state.Insert(a.replacement)
	a.find(false)
}

func (a *App) replaceAll() {
	if a.busy != "" || a.query == "" {
		return
	}
	snapshot := a.doc.Buffer.Snapshot()
	query, replacement, buffer := a.searchQuery(), a.replacement, a.doc.Buffer
	ctx, id := a.startJob("正在全部替换…")
	go func() {
		text, count, err := search.ReplaceAll(ctx, snapshot, query, replacement)
		if err == nil {
			err = ctx.Err()
		}
		if err == nil && count > 0 {
			if buffer.Version() != snapshot.Version() {
				err = fmt.Errorf("文本已改变，请重新执行替换")
			} else {
				buffer.Set(text)
			}
		}
		a.finishJob(id, func() {
			if err != nil {
				a.fail(err)
				return
			}
			a.searchStatus = fmt.Sprintf("已替换 %s 处", formatNumber(count))
			if count > 0 {
				a.state.Select(0, 0)
			}
		})
	}()
}
