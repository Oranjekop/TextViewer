package desktop

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

const recentFileLimit = 10

func normalizeRecentFiles(paths []string) []string {
	var recent []string
	for _, path := range paths {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			continue
		}
		path = filepath.Clean(path)
		if slices.ContainsFunc(recent, func(other string) bool { return sameResolvedPath(path, other) }) {
			continue
		}
		recent = append(recent, path)
		if len(recent) == recentFileLimit {
			break
		}
	}
	return recent
}

func (a *App) rememberRecentFile(path string) {
	recent := normalizeRecentFiles(append([]string{path}, a.settings.RecentFiles...))
	if slices.Equal(recent, a.settings.RecentFiles) {
		return
	}
	a.settings.RecentFiles = recent
	a.persistPreferences()
}

func (a *App) clearRecentFiles() {
	a.settings.RecentFiles = nil
	a.persistPreferences()
}

func recentFileLabel(index int, path string) string {
	runes := []rune(path)
	if len(runes) > 64 {
		path = string(runes[:16]) + "…" + string(runes[len(runes)-47:])
	}
	return fmt.Sprintf("%d. %s", index+1, path)
}

func (a *App) recentFilesMenu(m *ui.Menu) {
	locked := a.busy != nil || a.confirming
	if len(a.settings.RecentFiles) == 0 {
		m.Item(locales.NoRecentFiles()).Disabled(true)
	}
	for index, path := range a.settings.RecentFiles {
		if m.Item(recentFileLabel(index, path)).Disabled(locked).Chosen() {
			a.openFiles([]string{path})
		}
	}
	m.Separator()
	if m.Item(locales.ClearRecentFiles()).Disabled(locked || len(a.settings.RecentFiles) == 0).Chosen() {
		a.clearRecentFiles()
	}
}
