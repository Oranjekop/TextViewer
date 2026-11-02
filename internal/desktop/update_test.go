package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/update"
	"github.com/egoist/mygo/ui"
)

func TestUpdateMenuAndCurrentVersion(t *testing.T) {
	a := testApp()
	a.version = "0.0.1"
	a.settings.AutoCheckUpdates = true
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"检查更新…", "启动时检查更新", "TextViewer v0.0.1"} {
		if _, ok := tester.Find(label); !ok {
			t.Fatalf("missing %q in Settings", label)
		}
	}
	_ = tester.Click("取消")
	a.availableUpdate = &update.Release{Version: "0.0.2"}
	tester.Frame()
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tester.Find("安装更新 v0.0.2…"); !ok {
		t.Fatal("downloaded/available update not reachable from Settings")
	}
}

func TestAutoUpdatePreferenceMigratesAndCanBeDisabled(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := preferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"fontSize":18,"wrap":false,"theme":"浅色"}`), 0600); err != nil {
		t.Fatal(err)
	}
	prefs := loadPreferences()
	if !prefs.AutoCheckUpdates || prefs.FontSize != 18 || prefs.Wrap {
		t.Fatal("old preferences did not migrate safely")
	}
	a := testApp()
	a.version, a.settings.AutoCheckUpdates = "0.0.1", true
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("启动时检查更新"); err != nil {
		t.Fatal(err)
	}
	if a.settings.AutoCheckUpdates != true {
		t.Fatal("draft opt-out applied before saving")
	}
	if err := tester.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	if a.settings.AutoCheckUpdates || loadPreferences().AutoCheckUpdates {
		t.Fatal("auto update opt-out did not persist")
	}
}

func TestAutomaticUpdateCheckIsThrottledAndToleratesClockChanges(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		last    time.Time
		due     bool
	}{
		{"disabled", false, time.Unix(0, 0), false},
		{"first launch", true, time.Unix(0, 0), true},
		{"checked recently", true, time.Now().Add(-time.Hour), false},
		{"checked yesterday", true, time.Now().Add(-25 * time.Hour), true},
		{"clock moved backwards", true, time.Now().Add(time.Hour), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp()
			a.settings.AutoCheckUpdates, a.settings.LastUpdateCheck = tc.enabled, tc.last.Unix()
			a.scheduleUpdateCheck()
			scheduled := a.updateCancel != nil
			a.stopUpdateCheck()
			if scheduled != tc.due {
				t.Fatalf("scheduled %v, want %v", scheduled, tc.due)
			}
		})
	}
}
