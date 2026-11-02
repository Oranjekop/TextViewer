package desktop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"github.com/Oranjekop/TextViewer/internal/update"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type App struct {
	Win                *mygo.Window
	doc                *document.Document
	state              *editor.State
	editor             ui.Handle
	queryInput         ui.Handle
	fontDialog         bool
	fontDraft          string
	fontOptions        []string
	settingsDialog     bool
	settingsDraft      settingsDraft
	settingsLoading    bool
	settingsReadFailed bool
	settingsLoadID     uint64
	settingsError      string
	integration        integrationBackend
	fontSize           string
	language           string
	wrap               bool
	findOpen           bool
	replacing          bool
	query              string
	replacement        string
	caseSensitive      bool
	searchStatus       string
	status             string
	busy               string
	job                uint64
	cancel             context.CancelFunc
	cancellable        bool
	title              string
	closed             bool
	pendingFocus       bool
	pendingOpen        string
	settings           preferences
	column             columnCache
	version            string
	updateClient       *update.Client
	updateCancel       context.CancelFunc
	updateChecking     bool
	updatePending      bool
	availableUpdate    *update.Release
	updateInstaller    *update.Installer
}

func New() *App {
	prefs := loadPreferences()
	a := &App{
		doc: document.New(), settings: prefs, fontSize: strconv.Itoa(prefs.FontSize), wrap: prefs.Wrap,
		version: mygo.App.Version(), updateClient: update.NewClient(), integration: windowsIntegration{},
	}
	a.resetEditor()
	return a
}

func (a *App) resetEditor() {
	a.language = languageAuto
	a.applyLanguage()
	a.state = &editor.State{
		Buffer:          a.doc.Buffer,
		SoftWrap:        a.wrap,
		NoAutoClose:     true,
		NoFolding:       true,
		NoIndentGuides:  true,
		NoScrollPastEnd: true,
		Whitespace:      editor.WhitespaceNone,
		Label:           "文本编辑区",
		ContextMenu:     a.editMenu,
		Bindings: []editor.Binding{
			{Mods: ui.Ctrl, Key: ui.KeyF, Command: editor.NoCommand},
			{Mods: ui.Ctrl, Key: ui.KeyH, Command: editor.NoCommand},
			{Key: ui.KeyF3, Command: editor.NoCommand},
			{Mods: ui.Shift, Key: ui.KeyF3, Command: editor.NoCommand},
		},
	}
	a.pendingFocus = true
}

func (a *App) Ready() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{}))
	a.Win = mygo.NewWindow(mygo.WindowOptions{
		Title: "TextViewer",
		Width: 1120, Height: 780,
		MinWidth: 760, MinHeight: 420,
		Content: ui.View(a.view),
	})
	a.Win.OnClose(func(e *mygo.CloseEvent) {
		if a.busy != "" {
			e.PreventDefault()
			a.status = "操作尚未完成，可以先取消"
			a.Win.Invalidate()
			return
		}
		if a.doc.Dirty() && a.doc.HasChanges() {
			e.PreventDefault()
			a.confirmLeave(a.Win.Destroy)
		}
	})
	a.Win.OnClosed(func() {
		a.closed = true
		a.stopJob()
		a.stopUpdateCheck()
		mygo.App.Quit()
	})
	a.Win.OnFileDrop(func(e *mygo.FileDropEvent) {
		if len(e.Paths) > 0 {
			a.open(e.Paths[0])
		}
	})
	a.updateTitle()
	a.scheduleUpdateCheck()
}

func (a *App) OpenArguments(args []string, dir string) {
	if a.Win == nil {
		return
	}
	a.Win.Show()
	a.Win.Focus()
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		if !filepath.IsAbs(arg) && dir != "" {
			arg = filepath.Join(dir, arg)
		}
		a.open(arg)
		return
	}
}

func (a *App) view(c *ui.Context) {
	if a.settings.Theme == "浅色" {
		c.SetTheme(ui.LightTheme())
	} else if a.settings.Theme == "深色" {
		c.SetTheme(ui.DarkTheme())
	}
	a.shortcuts(c)
	t := c.Theme()
	fontSize, _ := strconv.Atoi(a.fontSize)
	code := a.doc.Buffer.Language() != nil
	a.state.Font = ui.Font{Family: a.editorFont(), Size: float32(fontSize)}
	a.state.NoIndentGuides = !code
	a.state.SoftWrap = a.wrap
	a.state.ReadOnly = a.busy != ""
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.toolbar(c)
		ui.Divider(c)
		if a.findOpen {
			a.findBar(c)
			ui.Divider(c)
		}
		in := editor.View(c.Key("document"), a.state).Fill().Bind(&a.editor)
		if a.pendingFocus {
			in.Focus()
			a.pendingFocus = false
		}
		ui.Divider(c)
		ui.Row(c).Height(30).Padding(0, 14).Gap(18).AlignItems(ui.Center).Background(t.Surface).Children(func() {
			status := a.status
			if a.busy != "" {
				status = a.busy
			}
			if status != "" {
				ui.Text(c, status).FontSize(12).TextColor(t.TextMuted).Grow(1)
			} else {
				ui.Text(c, fmt.Sprintf("%s  ·  %s 行", formatSize(a.doc.Buffer.Len()), formatNumber(a.doc.Buffer.LineCount()))).FontSize(12).TextColor(t.TextMuted).Grow(1)
			}
			if a.busy != "" && a.cancellable {
				if ui.Button(c, "取消").Height(24).Clicked() && a.cancel != nil {
					a.cancel()
				}
			}
			pos := a.state.CursorPos()
			snapshot := a.doc.Buffer.Snapshot()
			column := a.column.at(snapshot, a.state.Cursor())
			ui.Text(c, fmt.Sprintf("行 %s，列 %s", formatNumber(pos.Line+1), formatNumber(column))).FontSize(12).TextColor(t.TextMuted)
			ui.Button(c.Key("encoding"), string(a.doc.Encoding)).Height(24).MinWidth(0).Padding(0, 6).
				FontSize(12).Label("编码").Tooltip("以所选编码保存").Disabled(a.busy != "").Menu(a.encodingMenu)
			ending := "LF"
			if a.doc.Buffer.LineEnding() == "\r\n" {
				ending = "CRLF"
			}
			ui.Button(c.Key("line-ending"), ending).Height(24).MinWidth(0).Padding(0, 6).
				FontSize(12).Label("换行格式").Tooltip(a.lineEndingLabel()).Disabled(a.busy != "").Menu(a.lineEndingMenu)
		})
	})
	hadModal := a.fontDialog || a.settingsDialog
	a.fontChooser(c)
	a.settingsWindow(c)
	if hadModal && !a.fontDialog && !a.settingsDialog {
		a.pendingFocus = true
		a.showUpdateIfReady()
	}
	a.updateTitle()
}

func (a *App) editMenu(m *ui.Menu) {
	readOnly := a.busy != ""
	for _, item := range []struct {
		label    string
		command  editor.Command
		disabled bool
	}{
		{"撤销", editor.Undo, readOnly || !a.doc.Buffer.CanUndo()},
		{"重做", editor.Redo, readOnly || !a.doc.Buffer.CanRedo()},
	} {
		if m.Item(item.label).Disabled(item.disabled).Chosen() {
			a.state.Run(item.command)
		}
	}
	m.Separator()
	if m.Item("查找").Shortcut(ui.Ctrl, ui.KeyF).Chosen() {
		a.showFind(false)
	}
	if m.Item("替换").Shortcut(ui.Ctrl, ui.KeyH).Chosen() {
		a.showFind(true)
	}
	m.Separator()
	a.jsonFormatMenuItem(m)
}

func (a *App) fileMenu(m *ui.Menu) {
	if m.Item("新建").Shortcut(ui.Ctrl, ui.KeyN).Disabled(a.busy != "").Chosen() {
		a.confirmLeave(func() {
			a.doc = document.New()
			a.resetEditor()
			a.status, a.searchStatus = "", ""
		})
	}
	if m.Item("打开…").Shortcut(ui.Ctrl, ui.KeyO).Disabled(a.busy != "").Chosen() {
		a.chooseOpen()
	}
	if m.Item("保存").Shortcut(ui.Ctrl, ui.KeyS).Disabled(a.busy != "").Chosen() {
		a.save(false, nil)
	}
	if m.Item("另存为…").Shortcut(ui.Ctrl|ui.Shift, ui.KeyS).Disabled(a.busy != "").Chosen() {
		a.save(true, nil)
	}
	m.Separator()
	if m.Item("退出").Chosen() {
		a.Win.Close()
	}
}

func (a *App) shortcuts(c *ui.Context) {
	if a.fontDialog || a.settingsDialog {
		return
	}
	c.OnShortcut(ui.Ctrl, ui.KeyO, a.chooseOpen)
	c.OnShortcut(ui.Ctrl, ui.KeyN, func() {
		if a.busy == "" {
			a.confirmLeave(func() { a.doc = document.New(); a.resetEditor(); a.status = "" })
		}
	})
	c.OnShortcut(ui.Ctrl, ui.KeyS, func() { a.save(false, nil) })
	c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyS, func() { a.save(true, nil) })
	c.OnShortcut(ui.Ctrl, ui.KeyF, func() { a.showFind(false) })
	c.OnShortcut(ui.Ctrl, ui.KeyH, func() { a.showFind(true) })
	c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyJ, a.formatJSON)
	c.OnShortcut(0, ui.KeyF3, func() { a.find(false) })
	c.OnShortcut(ui.Shift, ui.KeyF3, func() { a.find(true) })
	c.OnShortcut(ui.Ctrl, ui.KeyEqual, func() { a.zoom(2) })
	c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyEqual, func() { a.zoom(2) })
	c.OnShortcut(ui.Ctrl, ui.KeyMinus, func() { a.zoom(-2) })
	c.OnShortcut(ui.Ctrl, ui.Key0, func() { a.fontSize = "16"; a.persistPreferences() })
	c.OnShortcut(ui.Alt, ui.KeyZ, func() { a.wrap = !a.wrap; a.persistPreferences() })
	if a.findOpen {
		c.OnShortcut(0, ui.KeyEscape, func() { a.findOpen = false; a.editor.Focus() })
	}
}

func (a *App) zoom(delta int) {
	n, _ := strconv.Atoi(a.fontSize)
	a.fontSize = strconv.Itoa(max(10, min(48, n+delta)))
	a.persistPreferences()
}

func (a *App) persistPreferences() {
	a.settings.FontSize, _ = strconv.Atoi(a.fontSize)
	a.settings.Wrap = a.wrap
	if err := savePreferences(a.settings); err != nil {
		a.status = "设置未保存：" + err.Error()
	}
}

func (a *App) chooseOpen() {
	if a.busy != "" {
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.Win, Title: "打开文本文件",
		Filters: []mygo.FileFilter{
			{Name: "文本和代码文件", Extensions: filetypes.Extensions()},
			{Name: "所有文件", Extensions: []string{"*"}},
		},
	})
	if err != nil {
		a.fail(err)
	} else if len(paths) > 0 {
		a.open(paths[0])
	}
}

func (a *App) open(path string) {
	if a.busy != "" {
		a.pendingOpen = path
		return
	}
	if a.doc.Path != "" && document.SamePath(a.doc.Path, path) {
		return
	}
	a.confirmLeave(func() {
		ctx, id := a.startJob("正在打开 " + filepath.Base(path) + "…")
		go func() {
			doc, err := document.Load(ctx, path)
			a.finishJob(id, func() {
				if err != nil {
					a.fail(err)
					return
				}
				a.doc = doc
				a.resetEditor()
				a.state.Select(0, 0)
				a.searchStatus = ""
				a.status = ""
			})
		}()
	})
}

func (a *App) confirmLeave(next func()) {
	if a.busy != "" {
		return
	}
	if !a.doc.Dirty() || !a.doc.HasChanges() {
		next()
		return
	}
	name := "未命名.txt"
	if a.doc.Path != "" {
		name = filepath.Base(a.doc.Path)
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageQuestion, Title: "TextViewer",
		Message: "是否保存对“" + name + "”的修改？",
		Buttons: []string{"保存", "不保存", "取消"}, DefaultButton: 0, CancelButton: 2,
	})
	if err != nil {
		a.fail(err)
		return
	}
	switch res.Button {
	case 0:
		a.save(false, next)
	case 1:
		next()
	}
}

func (a *App) save(saveAs bool, then func()) {
	if a.busy != "" {
		return
	}
	path := a.doc.Path
	if path == "" || saveAs {
		if path == "" {
			path = "未命名.txt"
		}
		chosen, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: a.Win, Title: "保存文本文件", DefaultPath: path,
			Filters: []mygo.FileFilter{{Name: "文本文件", Extensions: []string{"txt"}}, {Name: "所有文件", Extensions: []string{"*"}}},
		})
		if err != nil {
			a.fail(err)
			return
		}
		if chosen == "" {
			return
		}
		path = chosen
	}
	snapshot := a.doc.CaptureSave()
	ctx, id := a.startJob("正在保存…")
	doc := a.doc
	go func() {
		stamp, err := doc.SaveSnapshot(ctx, path, snapshot)
		a.finishJob(id, func() {
			if err != nil {
				a.fail(err)
				return
			}
			doc.AcceptSave(path, snapshot, stamp)
			a.applyLanguage()
			a.status = ""
			if then != nil {
				then()
			}
		})
	}()
}

func (a *App) startJob(label string) (context.Context, uint64) {
	a.stopJob()
	a.job++
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.busy = cancel, label
	a.cancellable = true
	a.state.ReadOnly = true
	return ctx, a.job
}

func (a *App) stopJob() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *App) finishJob(id uint64, fn func()) {
	a.Win.Update(func() {
		if a.closed || a.job != id {
			return
		}
		a.stopJob()
		a.busy = ""
		a.state.ReadOnly = false
		fn()
		if !a.closed && a.busy == "" && a.pendingOpen != "" {
			path := a.pendingOpen
			a.pendingOpen = ""
			a.open(path)
		}
		a.showUpdateIfReady()
	})
}

func (a *App) fail(err error) {
	if errors.Is(err, context.Canceled) {
		a.status = "已取消"
		return
	}
	a.status = err.Error()
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{
		Parent: a.Win, Type: mygo.MessageError, Title: "TextViewer",
		Message: "操作未完成", Detail: err.Error(), Buttons: []string{"确定"},
	})
}

func (a *App) updateTitle() {
	name := "未命名.txt"
	if a.doc.Path != "" {
		name = filepath.Base(a.doc.Path)
	}
	if a.doc.Dirty() {
		name = "* " + name
	}
	title := name + " - TextViewer"
	if title != a.title {
		a.title = title
		if a.Win != nil {
			a.Win.SetTitle(title)
		}
	}
}

func formatSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
}

func formatNumber(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

type columnCache struct {
	snapshot editor.Snapshot
	line     int
	offset   int
	column   int
	valid    bool
}

func (c *columnCache) at(snapshot editor.Snapshot, offset int) int {
	offset = max(0, min(offset, snapshot.Len()))
	line := snapshot.LineOf(offset)
	start, column := snapshot.LineStart(line), 1
	if c.valid && c.snapshot == snapshot && c.line == line {
		start, column = c.offset, c.column
	}
	delta := 0
	snapshot.Chunks(min(start, offset), max(start, offset), func(chunk string) bool {
		delta += utf8.RuneCountInString(chunk)
		return true
	})
	if offset < start {
		column -= delta
	} else {
		column += delta
	}
	*c = columnCache{snapshot: snapshot, line: line, offset: offset, column: column, valid: true}
	return column
}
