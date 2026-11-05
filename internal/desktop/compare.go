package desktop

import (
	"context"
	"slices"
	"strconv"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	maxCompareBytes = 8 << 20
	maxCompareLines = 50000
)

type textComparison struct {
	diff                  editor.DiffState
	leftName, rightName   string
	leftPath, rightPath   string
	added, deleted, hunks int
}

func (a *App) chooseComparison() {
	if a.busy != nil || a.confirming || len(a.tabs) < 2 {
		return
	}
	a.compareLeft = a.documentTab
	a.compareRight = a.tabs[(a.tabIndex+1)%len(a.tabs)]
	a.compareDialog = true
}

func (a *App) comparisonChooser(c *ui.Context) {
	ui.Modal(c.Key("compare-dialog"), &a.compareDialog, func() {
		ui.Text(c, locales.CompareText()).FontSize(18).Bold()
		for _, side := range []struct {
			label string
			tab   **documentTab
		}{
			{locales.CompareLeft(), &a.compareLeft},
			{locales.CompareRight(), &a.compareRight},
		} {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, side.label).Width(64)
				label := ""
				if *side.tab != nil {
					label = (*side.tab).name()
				}
				ui.MenuButton(c, label, func(m *ui.Menu) {
					for i, tab := range a.tabs {
						if m.Item(strconv.Itoa(i+1) + ". " + tab.tooltip()).Radio(*side.tab == tab).Chosen() {
							*side.tab = tab
						}
					}
				}).Width(284).Label(side.label)
			})
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.compareDialog = false
			}
			if ui.PrimaryButton(c, locales.StartComparison()).Disabled(a.compareLeft == a.compareRight).Clicked() {
				a.startComparison()
			}
		})
	}).Width(408)
	if !a.compareDialog {
		a.compareLeft, a.compareRight = nil, nil
	}
}

// Build from immutable snapshots, not the live buffers: viewing a comparison
// must never change source selections, undo histories or saved formats.
func prepareComparison(ctx context.Context, left, right editor.Snapshot) (*textComparison, error) {
	for _, snapshot := range []editor.Snapshot{left, right} {
		if snapshot.Len() > maxCompareBytes || snapshot.LineCount() > maxCompareLines {
			return nil, locales.NewError(locales.CompareTooLarge)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &textComparison{diff: editor.DiffState{
		Old: editor.NewBuffer(left.Slice(0, left.Len())),
		New: editor.NewBuffer(right.Slice(0, right.Len())),
		// Line highlighting avoids unbounded word-diff work on very long lines.
		Inline: editor.DiffInlineNone,
	}}
	result.added, result.deleted = result.diff.Stats()
	result.hunks = len(result.diff.Hunks())
	return result, ctx.Err()
}

func (a *App) startComparison() {
	left, right := a.compareLeft, a.compareRight
	if a.busy != nil || a.confirming || left == nil || right == nil || left == right ||
		!slices.Contains(a.tabs, left) || !slices.Contains(a.tabs, right) {
		return
	}
	leftSnapshot, rightSnapshot := left.doc.Buffer.Snapshot(), right.doc.Buffer.Snapshot()
	leftName, rightName := left.name(), right.name()
	leftPath, rightPath := left.tooltip(), right.tooltip()
	a.compareDialog = false
	a.compareLeft, a.compareRight = nil, nil
	a.preview.release()
	a.wheel = editorWheel{}
	ctx, id := a.startJob(locales.ComparingText)
	a.background(func() {
		result, err := prepareComparison(ctx, leftSnapshot, rightSnapshot)
		a.updateUI(func() {
			if a.closed || a.job != id {
				return
			}
			if err == nil {
				err = ctx.Err()
			}
			a.finishJob(id, func() {
				if err != nil {
					a.fail(err)
					return
				}
				result.leftName, result.rightName = leftName, rightName
				result.leftPath, result.rightPath = leftPath, rightPath
				a.comparison = result
			})
		})
	})
}

func (a *App) closeComparison() {
	a.comparison = nil
	a.compareLeft, a.compareRight = nil, nil
	a.pendingFocus = true
	a.showUpdateIfReady()
}

func (a *App) stepComparison(delta int) {
	result := a.comparison
	if result == nil || result.hunks == 0 {
		return
	}
	index, inside := result.diff.CurrentHunk()
	if delta > 0 {
		index++
	} else if inside {
		index--
	}
	result.diff.GoToHunk((index + result.hunks) % result.hunks)
}

func (a *App) comparisonView(c *ui.Context) {
	result := a.comparison
	c.OnShortcut(0, ui.KeyEscape, a.closeComparison)
	c.OnShortcut(ui.Cmd, ui.KeyW, a.closeComparison)
	c.OnShortcut(0, ui.KeyF3, func() { a.stepComparison(1) })
	c.OnShortcut(ui.Shift, ui.KeyF3, func() { a.stepComparison(-1) })
	fontSize, _ := strconv.Atoi(a.fontSize)
	result.diff.Font = ui.Font{Family: a.editorFont(), Size: float32(fontSize)}
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		ui.Row(c).FillWidth().Height(48).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.CompareText()).Bold().Grow(1)
			if ui.Button(c, locales.PreviousDifference()).Disabled(result.hunks == 0).Clicked() {
				a.stepComparison(-1)
			}
			if ui.Button(c, locales.NextDifference()).Disabled(result.hunks == 0).Clicked() {
				a.stepComparison(1)
			}
			if ui.Button(c, locales.CloseComparison()).Clicked() {
				a.closeComparison()
			}
		})
		ui.Divider(c)
		ui.Row(c).FillWidth().Height(32).Padding(0, 12).Gap(12).Children(func() {
			ui.Text(c, result.leftName).Grow(1).MinWidth(0).SingleLine().Tooltip(result.leftPath)
			ui.Text(c, result.rightName).Grow(1).MinWidth(0).SingleLine().Tooltip(result.rightPath)
		})
		editor.DiffView(c.Key("text-comparison"), &result.diff).FillWidth().Grow(1).MinHeight(0).Label(locales.CompareText())
		ui.Divider(c)
		ui.Text(c, locales.ComparisonStats(result.hunks, result.added, result.deleted)).Height(30).Padding(4, 12)
	})
}
