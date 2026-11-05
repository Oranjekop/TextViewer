package desktop

import (
	"runtime"
	"strings"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

func contextMenuLabel() string {
	if runtime.GOOS == "darwin" {
		return locales.FinderQuickAction()
	}
	return locales.ContextMenu()
}

func findReplaceShortcut() (ui.Modifiers, ui.Key) {
	if runtime.GOOS == "darwin" {
		return ui.Cmd | ui.Alt, ui.KeyF
	}
	return ui.Cmd, ui.KeyH
}

func shortcutText(text string) string {
	if runtime.GOOS != "darwin" {
		return text
	}
	if text == "Ctrl+H" {
		return "Cmd+Option+F"
	}
	if text == "Ctrl+Y" {
		return "Cmd+Shift+Z"
	}
	return strings.ReplaceAll(text, "Ctrl+", "Cmd+")
}

func defaultTextFont() string {
	if runtime.GOOS == "darwin" {
		return "sans-serif"
	}
	return "Microsoft YaHei UI"
}
