package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type documentTab struct {
	id           uint64
	untitled     int
	doc          *document.Document
	state        *editor.State
	wheel        editorWheel
	language     string
	column       columnCache
	searchStatus locales.Message
	searchFailed bool
	searchResult searchResult
	preview      markdownPreview
}

func (tab *documentTab) name() string {
	if tab.doc.Path != "" {
		return filepath.Base(tab.doc.Path)
	}
	if tab.untitled <= 1 {
		return locales.Untitled()
	}
	return locales.UntitledNumber(strconv.Itoa(tab.untitled))
}

func (tab *documentTab) tooltip() string {
	if tab.doc.Path != "" {
		return tab.doc.Path
	}
	return tab.name()
}

func (a *App) addDocument(doc *document.Document) {
	if a.documentTab != nil {
		a.wheel = editorWheel{}
		a.preview.release()
	}
	a.nextTabID++
	tab := &documentTab{id: a.nextTabID, doc: doc}
	if doc.Path == "" {
		a.untitledCount++
		tab.untitled = a.untitledCount
	}
	a.tabs = append(a.tabs, tab)
	a.tabIndex = len(a.tabs) - 1
	a.documentTab = tab
	a.resetEditor()
	a.status, a.revealTab = nil, true
}

func (a *App) newDocument() {
	if a.busy == nil && !a.confirming {
		a.addDocument(document.New())
	}
}

func (a *App) activateTab(tab *documentTab) {
	if a.busy != nil || a.confirming || tab == a.documentTab || !slices.Contains(a.tabs, tab) {
		return
	}
	a.preview.release()
	a.wheel = editorWheel{}
	a.documentTab = tab
	a.tabIndex = slices.Index(a.tabs, tab)
	a.status = nil
	a.pendingFocus, a.revealTab = true, true
}

func (a *App) nextTab(delta int) {
	if len(a.tabs) < 2 {
		return
	}
	index := slices.Index(a.tabs, a.documentTab)
	a.activateTab(a.tabs[(index+delta+len(a.tabs))%len(a.tabs)])
}

func (a *App) tabBar(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	reveal := a.revealTab || width != a.tabWindowWidth
	a.tabWindowWidth = width
	var choose, close *documentTab
	keyboardChoice := false
	ui.Row(c).FillWidth().Height(40).Gap(4).Padding(0, 8).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		ui.ScrollHorizontal(c.Key("tabs")).Grow(1).MinWidth(0).Height(40).Scrollbars(false).Children(func() {
			parts := ui.TabsBase(c, &a.tabIndex, len(a.tabs))
			parts.List.Height(40).Gap(t.Space(1)).Disabled(a.busy != nil).Children(func() {
				for i, docTab := range a.tabs {
					selected := i == a.tabIndex
					tab := parts.Tab(i).Height(40).MinWidth(100).MaxWidth(240).
						Padding(t.Space(2), t.Space(3)).Gap(8).AlignItems(ui.Center).FocusRing(false).
						Label(locales.TabLabel(docTab.name())).Tooltip(docTab.tooltip()).TextColor(t.TextMuted)
					if selected {
						tab.TextColor(t.Text).FontWeight(600)
						keyboardChoice = tab.FocusVisible()
					} else if tab.Hovered() {
						tab.TextColor(t.Text)
					}
					// Match MyGo's Tabs appearance while adding a close action.
					tab.DrawOver(func(p *ui.Painter, r ui.Rect) {
						if selected {
							in, height := t.Space(1.5), t.Space(0.5)
							p.Fill(ui.Rect{X: r.X + in, Y: r.Y + r.H - height, W: r.W - 2*in, H: height}, t.Accent, height/2)
						}
						if tab.FocusVisible() {
							p.FocusRing(r, [4]float32{t.Radius, t.Radius, t.Radius, t.Radius})
						}
					})
					if selected && reveal {
						tab.ScrollIntoView()
					}
					tab.Children(func() {
						label := docTab.name()
						if docTab.doc.Dirty() {
							label = "* " + label
						}
						ui.Text(c, label).SingleLine().Grow(1).MinWidth(0).FontSize(13)
						if tabAction(c, "close", locales.CloseTabLabel(docTab.name()), locales.CloseTab()+" (Ctrl+W)").Clicked() {
							close = docTab
						}
					})
				}
			})
			if parts.List.Changed() {
				choose = a.tabs[a.tabIndex]
			}
		})
		if a.isMarkdown() {
			preview := tabAction(c, "preview", locales.MarkdownPreview(), locales.MarkdownPreview()+" (Ctrl+Shift+V)").
				Size(28, 28).Disabled(a.busy != nil || a.confirming)
			if a.preview.open {
				preview.Background(t.SurfacePressed).TextColor(t.Accent)
			}
			if preview.Clicked() {
				a.toggleMarkdownPreview()
			}
		}
		if len(a.tabs) > 1 {
			tabAction(c, "chevron-down", locales.TabList(), locales.OpenFiles()).Size(28, 28).
				Disabled(a.busy != nil).Menu(a.tabMenu)
		}
		if tabAction(c, "plus", locales.NewTab(), locales.NewFile()+" (Ctrl+N)").Size(28, 28).Disabled(a.busy != nil).Clicked() {
			a.newDocument()
		}
	})
	a.revealTab = false
	if choose != nil {
		a.activateTab(choose)
		if keyboardChoice {
			a.pendingFocus = false
		}
	}
	if close != nil {
		a.closeTab(close)
	}
}

func tabAction(c *ui.Context, icon, label, tip string) ui.Element {
	t := c.Theme()
	button := ui.ButtonBase(c).Size(20, 20).MinWidth(20).Padding(0).Radius(t.Radius).
		Label(label).Tooltip(tip).TextColor(t.TextMuted).Children(func() {
		ui.Icon(c, toolbarIcons[icon]).Size(14, 14)
	})
	if button.Hovered() || button.FocusVisible() {
		button.Background(t.SurfaceHover).TextColor(t.Text)
	}
	return button
}

func (a *App) tabMenu(m *ui.Menu) {
	for _, tab := range a.tabs {
		if m.Item(tab.tooltip()).Radio(tab == a.documentTab).Chosen() {
			a.activateTab(tab)
		}
	}
}

func (a *App) closeTab(tab *documentTab) {
	if a.busy != nil || a.confirming || !slices.Contains(a.tabs, tab) {
		return
	}
	if tab.doc.Dirty() {
		a.activateTab(tab)
		a.confirmLeave(func() { a.removeTab(tab) })
	} else {
		a.removeTab(tab)
	}
}

func (a *App) removeTab(tab *documentTab) {
	index := slices.Index(a.tabs, tab)
	if index < 0 {
		return
	}
	tab.preview.release()
	a.tabs = slices.Delete(a.tabs, index, index+1)
	if len(a.tabs) == 0 {
		a.addDocument(document.New())
	} else if a.documentTab == tab {
		a.documentTab = a.tabs[min(index, len(a.tabs)-1)]
	}
	a.tabIndex = slices.Index(a.tabs, a.documentTab)
	a.status = nil
	a.pendingFocus, a.revealTab = true, true
}

func (a *App) hasDirtyTabs() bool {
	for _, tab := range a.tabs {
		if tab.doc.Dirty() {
			return true
		}
	}
	return false
}

func (a *App) confirmAllTabs(next func()) {
	tabs := slices.Clone(a.tabs)
	var confirm func(int)
	confirm = func(index int) {
		for index < len(tabs) {
			tab := tabs[index]
			if slices.Contains(a.tabs, tab) && tab.doc.Dirty() {
				a.activateTab(tab)
				a.confirmLeave(func() { confirm(index + 1) })
				return
			}
			index++
		}
		next()
	}
	confirm(0)
}

func (a *App) openFiles(paths []string) {
	for _, path := range paths {
		if path != "" {
			a.pendingOpen = append(a.pendingOpen, path)
		}
	}
	a.openNextFile()
}

func sameResolvedPath(first, second string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func findOpenTab(tabs []*documentTab, path string) *documentTab {
	for _, tab := range tabs {
		if tab.doc.Path != "" && sameResolvedPath(tab.doc.Path, path) {
			return tab
		}
	}
	return nil
}

func (a *App) openNextFile() {
	for !a.closed && !a.confirming && a.busy == nil && len(a.pendingOpen) > 0 {
		path := a.pendingOpen[0]
		a.pendingOpen[0] = ""
		a.pendingOpen = a.pendingOpen[1:]
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if tab := findOpenTab(a.tabs, path); tab != nil {
			a.activateTab(tab)
			continue
		}
		tabs := slices.Clone(a.tabs)
		name := filepath.Base(path)
		ctx, id := a.startJob(func() string { return locales.Opening(name) })
		a.background(func() {
			// Resolve junctions once off the UI thread. Existing document paths
			// are already resolved, so a batch does not repeat disk lookups for
			// every pair of open files.
			resolved, err := document.ResolvePath(path)
			var existing *documentTab
			var doc *document.Document
			if err == nil {
				existing = findOpenTab(tabs, resolved)
				if existing == nil {
					doc, err = document.Load(ctx, resolved)
				}
			}
			a.finishJob(id, func() {
				if err == nil {
					err = ctx.Err()
				}
				if err != nil {
					a.fail(err)
					if errors.Is(err, context.Canceled) {
						a.pendingOpen = nil
					}
					return
				}
				if existing != nil {
					a.activateTab(existing)
					return
				}
				if len(a.tabs) == 1 && a.doc.Path == "" && a.doc.Buffer.Len() == 0 && !a.doc.Dirty() {
					a.doc = doc
					a.resetEditor()
					a.status, a.searchStatus = nil, nil
				} else {
					a.addDocument(doc)
				}
			})
		})
		return
	}
}
