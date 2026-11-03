package desktop

import (
	"embed"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

//go:embed icons/*.svg
var toolbarSVG embed.FS

var toolbarIcons = loadToolbarIcons()

func loadToolbarIcons() map[string]*ui.SVG {
	names := []string{"file", "open", "save", "save-as", "edit", "undo", "redo", "find", "replace", "wrap", "font", "code", "settings", "chevron-down", "close", "plus"}
	icons := make(map[string]*ui.SVG, len(names))
	for _, name := range names {
		data, err := toolbarSVG.ReadFile("icons/" + name + ".svg")
		if err != nil {
			panic(err)
		}
		icons[name] = ui.MustParseSVG(data)
	}
	return icons
}

func iconButton(c *ui.Context, name, label, tip string, active bool) ui.Element {
	b := ui.Button(c.Key(label), "").Size(36, 34).MinWidth(36).Padding(0).
		Label(label).Tooltip(tip).Children(func() {
		ui.Icon(c, toolbarIcons[name]).Size(20, 20)
	})
	if active {
		t := c.Theme()
		b.Background(t.SurfacePressed).TextColor(t.Accent)
	}
	return b
}

func dropdownButton(c *ui.Context, text, label, tip string) ui.Element {
	return ui.Button(c, "").Height(24).MinWidth(0).Padding(0, 6).Gap(4).
		Label(label).Tooltip(tip).Children(func() {
		ui.Text(c, text).FontSize(12).SingleLine()
		ui.Icon(c, toolbarIcons["chevron-down"]).Size(10, 10).TextColor(c.Theme().TextMuted)
	})
}

func (a *App) toolbar(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	gap := float32(6)
	if width < 840 {
		gap = 2
	}
	ui.Toolbar(c, func() {
		ui.Button(c.Key("file"), "").Size(50, 34).MinWidth(50).Padding(0, 6).Gap(4).
			Label(locales.File()).Tooltip(locales.File()).Children(func() {
			ui.Icon(c, toolbarIcons["file"]).Size(20, 20)
			ui.Icon(c, toolbarIcons["chevron-down"]).Size(12, 12).TextColor(t.TextMuted)
		}).Menu(a.fileMenu)
		if iconButton(c, "open", locales.Open(), locales.Open()+" (Ctrl+O)", false).Disabled(a.busy != nil).Clicked() {
			a.chooseOpen()
		}
		if iconButton(c, "save", locales.Save(), locales.Save()+" (Ctrl+S)", false).Disabled(a.busy != nil).Clicked() {
			a.save(false, nil)
		}
		if iconButton(c, "save-as", locales.SaveAs(), locales.SaveAs()+" (Ctrl+Shift+S)", false).Disabled(a.busy != nil).Clicked() {
			a.save(true, nil)
		}
		ui.Button(c.Key("edit"), "").Size(50, 34).MinWidth(50).Padding(0, 6).Gap(4).
			Label(locales.Edit()).Tooltip(locales.Edit()).Disabled(a.busy != nil).Children(func() {
			ui.Icon(c, toolbarIcons["edit"]).Size(20, 20)
			ui.Icon(c, toolbarIcons["chevron-down"]).Size(12, 12).TextColor(t.TextMuted)
		}).Menu(a.editingMenu)
		locked := a.busy != nil || a.confirming
		if iconButton(c, "undo", locales.Undo(), locales.Undo()+" (Ctrl+Z)", false).
			Disabled(locked || !a.doc.Buffer.CanUndo()).Clicked() {
			a.runEditorCommand(editor.Undo)
		}
		if iconButton(c, "redo", locales.Redo(), locales.Redo()+" (Ctrl+Y)", false).
			Disabled(locked || !a.doc.Buffer.CanRedo()).Clicked() {
			a.runEditorCommand(editor.Redo)
		}
		ui.Box(c).Width(1).Height(20).Margin(0, 4).Background(t.Border)
		if iconButton(c, "find", locales.Find(), locales.Find()+" (Ctrl+F)", a.findOpen && !a.replacing).Clicked() {
			a.showFind(false)
		}
		if iconButton(c, "replace", locales.Replace(), locales.Replace()+" (Ctrl+H)", a.findOpen && a.replacing).Clicked() {
			a.showFind(true)
		}
		ui.Spacer(c)
		wrap := ui.Toggle(c.Key("wrap"), &a.wrap, "").Size(36, 34).MinWidth(36).Padding(0).
			Label(locales.WordWrap()).Tooltip(locales.WordWrap() + " (Alt+Z)").Children(func() {
			ui.Icon(c, toolbarIcons["wrap"]).Size(20, 20)
		})
		if a.wrap {
			wrap.Background(t.Accent.Alpha(0.12)).TextColor(t.Accent)
		}
		if wrap.Changed() {
			a.persistPreferences()
		}
		if iconButton(c, "font", locales.Font(), locales.FontTip(a.editorFont()), false).Clicked() {
			a.chooseFont()
		}
		if ui.Select(c, &a.fontSize, []string{"10", "12", "14", "16", "18", "20", "24", "28", "32", "40", "48"}).
			Width(68).MinWidth(0).Label(locales.FontSize()).Tooltip(locales.FontSizeTip()).Changed() {
			a.persistPreferences()
		}
		ui.Button(c.Key("language"), "").Width(164).MinWidth(164).Height(34).Padding(0, 10).
			Label(locales.CodeFormat()).Tooltip(a.languageTooltip()).Disabled(a.busy != nil).Children(func() {
			ui.Icon(c, toolbarIcons["code"]).Size(18, 18).TextColor(t.TextMuted)
			ui.Text(c, a.languageLabel()).SingleLine().Grow(1).FontSize(13)
			ui.Icon(c, toolbarIcons["chevron-down"]).Size(12, 12).TextColor(t.TextMuted)
		}).Menu(a.languageMenu)
		if iconButton(c, "settings", locales.Settings(), locales.Settings(), false).Disabled(a.busy != nil).Clicked() {
			a.chooseSettings()
		}
	}).FillWidth().Height(54).Padding(10, 14).Gap(gap).Background(t.Surface).Label(locales.Toolbar())
}
