package desktop

import (
	"os"
	"strings"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/i18n"
)

func (a *App) applyStartupLocale() {
	// MyGo's one-run development override must win over saved preferences.
	if strings.TrimSpace(os.Getenv("MYGO_LOCALE")) == "" {
		a.setLocale(a.settings.Locale)
	}
}

func normalizeLocale(tag string) string {
	switch tag = i18n.Normalize(tag); {
	case tag == "zh" || strings.HasPrefix(tag, "zh-"):
		return "zh-Hans"
	case tag == "en" || strings.HasPrefix(tag, "en-"):
		return "en"
	default:
		return ""
	}
}

func localeLabel(tag string) string {
	switch tag {
	case "zh-Hans":
		return "简体中文"
	case "en":
		return "English"
	default:
		return locales.FollowSystem()
	}
}

func themeLabel(theme string) string {
	switch theme {
	case "浅色":
		return locales.Light()
	case "深色":
		return locales.Dark()
	default:
		return locales.FollowSystem()
	}
}

func (a *App) refreshLocale() {
	for _, tab := range a.tabs {
		tab.state.Label = locales.EditorArea()
	}
	if a.fontDialog {
		a.refreshFontOptions()
	}
	if a.Win != nil {
		a.updateTitle()
	}
}
