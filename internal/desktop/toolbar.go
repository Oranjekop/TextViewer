package desktop

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

//go:embed icons/*.svg
var toolbarSVG embed.FS

var toolbarIcons = loadToolbarIcons()

func loadToolbarIcons() map[string]*ui.SVG {
	names := []string{"file", "open", "save", "save-as", "find", "replace", "wrap", "font", "code", "settings", "chevron-down", "close", "plus"}
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
	ui.Toolbar(c, func() {
		ui.Button(c.Key("file"), "").Size(50, 34).MinWidth(50).Padding(0, 6).Gap(4).
			Label("文件").Tooltip("文件").Children(func() {
			ui.Icon(c, toolbarIcons["file"]).Size(20, 20)
			ui.Icon(c, toolbarIcons["chevron-down"]).Size(12, 12).TextColor(t.TextMuted)
		}).Menu(a.fileMenu)
		if iconButton(c, "open", "打开", "打开 (Ctrl+O)", false).Disabled(a.busy != "").Clicked() {
			a.chooseOpen()
		}
		if iconButton(c, "save", "保存", "保存 (Ctrl+S)", false).Disabled(a.busy != "").Clicked() {
			a.save(false, nil)
		}
		if iconButton(c, "save-as", "另存为", "另存为 (Ctrl+Shift+S)", false).Disabled(a.busy != "").Clicked() {
			a.save(true, nil)
		}
		ui.Box(c).Width(1).Height(20).Margin(0, 4).Background(t.Border)
		if iconButton(c, "find", "查找", "查找 (Ctrl+F)", a.findOpen && !a.replacing).Clicked() {
			a.showFind(false)
		}
		if iconButton(c, "replace", "替换", "替换 (Ctrl+H)", a.findOpen && a.replacing).Clicked() {
			a.showFind(true)
		}
		ui.Spacer(c)
		wrap := ui.Toggle(c.Key("wrap"), &a.wrap, "").Size(36, 34).MinWidth(36).Padding(0).
			Label("自动换行").Tooltip("自动换行 (Alt+Z)").Children(func() {
			ui.Icon(c, toolbarIcons["wrap"]).Size(20, 20)
		})
		if a.wrap {
			wrap.Background(t.Accent.Alpha(0.12)).TextColor(t.Accent)
		}
		if wrap.Changed() {
			a.persistPreferences()
		}
		if iconButton(c, "font", "字体", "字体："+a.editorFont(), false).Clicked() {
			a.chooseFont()
		}
		if ui.Select(c, &a.fontSize, []string{"10", "12", "14", "16", "18", "20", "24", "28", "32", "40", "48"}).
			Width(68).MinWidth(0).Label("字号").Tooltip("字号 (Ctrl+加号 / 减号)").Changed() {
			a.persistPreferences()
		}
		ui.Button(c.Key("language"), "").Width(164).MinWidth(164).Height(34).Padding(0, 10).
			Label("代码格式").Tooltip(a.languageTooltip()).Disabled(a.busy != "").Children(func() {
			ui.Icon(c, toolbarIcons["code"]).Size(18, 18).TextColor(t.TextMuted)
			ui.Text(c, a.languageLabel()).SingleLine().Grow(1).FontSize(13)
			ui.Icon(c, toolbarIcons["chevron-down"]).Size(12, 12).TextColor(t.TextMuted)
		}).Menu(a.languageMenu)
		if iconButton(c, "settings", "设置", "设置", false).Disabled(a.busy != "").Clicked() {
			a.chooseSettings()
		}
	}).FillWidth().Height(54).Padding(10, 14).Gap(6).Background(t.Surface).Label("工具栏")
}
