package desktop

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/search"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type searchResult struct {
	snapshot editor.Snapshot
	query    search.Query
	match    search.Match
	valid    bool
}

func (a *App) showFind(replacing bool) {
	a.findOpen, a.replacing = true, replacing
	sel := a.state.Selection()
	result := a.searchResult
	isSearchMatch := result.valid && result.snapshot == a.doc.Buffer.Snapshot() &&
		result.query == a.searchQuery() && result.match == (search.Match{Start: sel.Start(), End: sel.End()})
	if size := sel.End() - sel.Start(); a.busy == nil && !isSearchMatch && size > 0 && size < 4096 {
		a.query = a.state.SelectedText()
		if a.regex {
			a.query = regexp.QuoteMeta(a.query)
		} else if a.escapeSequences {
			a.query = strings.ReplaceAll(a.query, `\`, `\\`)
		}
		if a.regex || a.escapeSequences {
			a.query = strings.NewReplacer("\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(a.query)
		}
		a.clearSearchResult()
	}
	a.queryInput.Focus()
}

func (a *App) clearSearchResult() {
	a.searchStatus, a.searchFailed = nil, false
	a.searchResult = searchResult{}
}

func (a *App) findBar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(10, 14).Gap(8).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.TextInput(c, &a.query).Grow(1).MinWidth(160).Placeholder(locales.Find()).Label(locales.QueryLabel()).Bind(&a.queryInput).
				Disabled(a.busy != nil).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Key == ui.KeyEnter {
						a.find(k.Mods&ui.Shift != 0)
						return true
					}
					return false
				}).Changed() {
				a.clearSearchResult()
			}
			if ui.Checkbox(c, &a.caseSensitive, locales.MatchCase()).Disabled(a.busy != nil).Changed() {
				a.clearSearchResult()
			}
			if ui.Checkbox(c, &a.regex, locales.Regex()).Disabled(a.busy != nil).
				Tooltip(locales.RegexTip()).Changed() {
				a.clearSearchResult()
			}
			escapes := a.escapeSequences || a.regex
			if ui.Checkbox(c, &escapes, locales.Escapes()).Disabled(a.busy != nil || a.regex).
				Tooltip(locales.EscapesTip()).Changed() {
				a.escapeSequences = escapes
				a.clearSearchResult()
			}
			if ui.Button(c, locales.Previous()).Disabled(a.busy != nil || a.query == "").Clicked() {
				a.find(true)
			}
			if ui.Button(c, locales.Next()).Disabled(a.busy != nil || a.query == "").Clicked() {
				a.find(false)
			}
			if ui.Button(c, locales.Close()).Label(locales.CloseFind()).Clicked() {
				a.findOpen = false
				a.editor.Focus()
			}
		})
		if a.replacing {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.TextInput(c, &a.replacement).Grow(1).MinWidth(160).Placeholder(locales.ReplaceWith()).Label(locales.ReplacementLabel()).
					Disabled(a.busy != nil).Tooltip(locales.ReplacementTip()).Changed() {
					a.clearSearchResult()
				}
				if ui.Button(c, locales.Replace()).Disabled(a.busy != nil || a.query == "").Clicked() {
					a.replaceOne()
				}
				if ui.Button(c, locales.ReplaceAll()).Disabled(a.busy != nil || a.query == "").Clicked() {
					a.replaceAll()
				}
			})
		}
		if a.searchStatus != nil {
			color := t.TextMuted
			if a.searchFailed {
				color = t.Danger
			}
			text := a.searchStatus.String()
			ui.Text(c, text).FontSize(12).TextColor(color).FillWidth().SingleLine().
				Label(locales.SearchResult()).Tooltip(text)
		}
	})
}

func (a *App) searchQuery() search.Query {
	return search.Query{Text: a.query, CaseSensitive: a.caseSensitive, Regex: a.regex, Escapes: a.escapeSequences}
}

func (a *App) searchError(err error) {
	a.searchResult = searchResult{}
	a.searchFailed = !errors.Is(err, context.Canceled)
	if a.searchFailed {
		a.searchStatus = err.Error
	} else {
		a.searchStatus = locales.Canceled
	}
}

func (a *App) find(backward bool) {
	if a.busy != nil || a.query == "" {
		return
	}
	snapshot := a.doc.Buffer.Snapshot()
	sel := a.state.Selection()
	at := sel.End()
	if backward {
		at = sel.Start()
	}
	query := a.searchQuery()
	previous := a.searchResult
	if !backward && previous.valid && previous.snapshot == snapshot && previous.query == query &&
		previous.match == (search.Match{Start: at, End: at}) {
		at = search.Advance(snapshot, at)
		if at > snapshot.Len() {
			at = 0
		}
	}
	ctx, id := a.startJob(locales.Finding)
	a.clearSearchResult()
	a.background(func() {
		match, ok, err := search.Next(ctx, snapshot, query, at, backward)
		a.finishJob(id, func() {
			if err == nil {
				err = ctx.Err()
			}
			if err == nil && a.doc.Buffer.Snapshot() != snapshot {
				err = locales.NewError(locales.SearchTextChanged)
			}
			if err != nil {
				a.searchError(err)
				return
			}
			if !ok {
				a.searchStatus = func() string { return locales.NotFound(query.Text) }
				return
			}
			a.state.Select(match.Start, match.End)
			a.searchStatus = nil
			a.searchResult = searchResult{snapshot: snapshot, query: query, match: match, valid: true}
		})
	})
}

func (a *App) replaceOne() {
	if a.busy != nil || a.query == "" {
		return
	}
	sel := a.state.Selection()
	snapshot := a.doc.Buffer.Snapshot()
	query, replacement, buffer := a.searchQuery(), a.replacement, a.doc.Buffer
	ctx, id := a.startJob(locales.Replacing)
	a.clearSearchResult()
	a.background(func() {
		text, exact, err := search.ReplaceSelection(ctx, snapshot, query, replacement, search.Match{Start: sel.Start(), End: sel.End()})
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
						a.searchError(err)
						return
					}
					if exact {
						at := sel.Start() + len(text)
						a.state.Select(at, at)
						if sel.Empty() {
							a.searchResult = searchResult{
								snapshot: buffer.Snapshot(), query: query,
								match: search.Match{Start: at, End: at}, valid: true,
							}
						}
					}
					a.find(false)
				})
			}
			if err != nil || !exact {
				finish()
				return
			}
			a.cancellable, a.busy = false, locales.ApplyingReplacement
			a.background(func() {
				err = applySearchEdit(ctx, buffer, snapshot, func() {
					buffer.Edit(editor.Edit{Start: sel.Start(), End: sel.End(), Text: text})
				})
				finish()
			})
		})
	})
}

func (a *App) replaceAll() {
	if a.busy != nil || a.query == "" {
		return
	}
	snapshot := a.doc.Buffer.Snapshot()
	query, replacement, buffer := a.searchQuery(), a.replacement, a.doc.Buffer
	ctx, id := a.startJob(locales.ReplacingAll)
	a.clearSearchResult()
	a.background(func() {
		text, count, err := search.ReplaceAll(ctx, snapshot, query, replacement)
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
						a.searchError(err)
						return
					}
					a.searchStatus = func() string { return locales.ReplacedCount(count) }
					if count > 0 {
						a.state.Select(0, 0)
					}
				})
			}
			if err != nil || count == 0 {
				finish()
				return
			}
			a.cancellable, a.busy = false, locales.ApplyingReplacement
			a.background(func() {
				err = applySearchEdit(ctx, buffer, snapshot, func() { buffer.Set(text) })
				finish()
			})
		})
	})
}

func applySearchEdit(ctx context.Context, buffer *editor.Buffer, snapshot editor.Snapshot, edit func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if buffer.Snapshot() != snapshot {
		return locales.NewError(locales.ReplacementTextChanged)
	}
	edit()
	return nil
}
