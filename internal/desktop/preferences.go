package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type preferences struct {
	FontFamily       string `json:"fontFamily,omitempty"`
	FontSize         int    `json:"fontSize"`
	Wrap             bool   `json:"wrap"`
	Theme            string `json:"theme"`
	Locale           string `json:"locale,omitempty"`
	AutoCheckUpdates bool   `json:"autoCheckUpdates"`
	LastUpdateCheck  int64  `json:"lastUpdateCheck,omitempty"`
	FileAssociations *bool  `json:"fileAssociations,omitempty"`
	ContextMenu      *bool  `json:"contextMenu,omitempty"`
}

func preferencesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "TextViewer", "settings.json")
}

func loadPreferences() preferences {
	p := preferences{FontSize: 16, Wrap: true, Theme: "跟随系统", AutoCheckUpdates: true}
	if data, err := os.ReadFile(preferencesPath()); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	if p.FontSize < 10 || p.FontSize > 48 {
		p.FontSize = 16
	}
	if p.Theme != "浅色" && p.Theme != "深色" {
		p.Theme = "跟随系统"
	}
	p.Locale = normalizeLocale(p.Locale)
	return p
}

func savePreferences(p preferences) error {
	path := preferencesPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
