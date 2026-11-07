package desktop

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/update"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

type fakeUpdateClient struct {
	check func(context.Context, string) (*update.Release, error)
	calls int
}

func (f *fakeUpdateClient) Check(ctx context.Context, version string) (*update.Release, error) {
	f.calls++
	return f.check(ctx, version)
}

func (*fakeUpdateClient) Download(context.Context, *update.Release, func(int64, int64)) (*update.Installer, error) {
	return nil, errors.New("unexpected update download")
}

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

func TestStartupUpdateCheckIgnoresLegacyTimestampAndHonorsOptOut(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		calls   int
	}{
		{"enabled", true, 2},
		{"disabled", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			path := preferencesPath()
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			config := `{"autoCheckUpdates":` + strconv.FormatBool(tc.enabled) +
				`,"lastUpdateCheck":` + strconv.FormatInt(time.Now().Unix(), 10) + `}`
			if err := os.WriteFile(path, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			release := &update.Release{Version: "0.0.6"}
			client := &fakeUpdateClient{}
			client.check = func(context.Context, string) (*update.Release, error) {
				if client.calls == 1 {
					return nil, errors.New("offline")
				}
				return release, nil
			}
			for launch := range 2 {
				a := testApp()
				a.settings = loadPreferences()
				a.version, a.updateClient = "0.0.5", client
				a.checkUpdatesOnStartup()
				if a.updateChecking || a.updateMessage != nil {
					t.Fatal("automatic check left a waiting state or reported a background error")
				}
				if tc.enabled && launch == 1 && (!a.updatePending || a.availableUpdate != release) {
					t.Fatal("relaunch after a failed check did not discover the update")
				}
			}
			if client.calls != tc.calls {
				t.Fatalf("got %d checks across two launches, want %d", client.calls, tc.calls)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != config {
				t.Fatalf("startup checks changed the saved settings: %v", err)
			}
		})
	}
}

func TestManualUpdateCheckKeepsSettingsAndReportsResultInline(t *testing.T) {
	for _, tc := range []struct {
		name    string
		release *update.Release
		err     error
		message string
	}{
		{"latest", nil, nil, "当前已是最新版本 v0.0.5"},
		{"failed", nil, errors.New("offline"), "暂时无法检查更新，请稍后再试。"},
		{"available", &update.Release{Version: "0.0.6"}, nil, "安装更新 v0.0.6…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APPDATA", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			a := testApp()
			a.version = "0.0.5"
			client := &fakeUpdateClient{}
			client.check = func(ctx context.Context, version string) (*update.Release, error) {
				if !a.settingsDialog || !a.updateChecking || version != a.version || ctx.Err() != nil {
					t.Fatal("check did not wait inside the open settings dialog")
				}
				a.checkUpdates(true)
				return tc.release, tc.err
			}
			a.updateClient = client
			tester := ui.NewTester(a.view, 760, 420)
			for _, label := range []string{"设置", "启动时检查更新", "检查更新…"} {
				if err := tester.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			if !a.settingsDialog || a.updateChecking || client.calls != 1 || a.updateCancel != nil ||
				!a.settingsDraft.AutoUpdate || a.settings.AutoCheckUpdates {
				t.Fatal("checking closed settings, changed its draft, or started duplicate requests")
			}
			if !tester.HasText(tc.message) || a.updateFailed != (tc.err != nil) || a.updatePending ||
				len(a.dialogs.(*fakeDialogs).messages) != 0 {
				t.Fatalf("result was not reported inline: message=%q, pending=%v", a.updateMessage, a.updatePending)
			}
			for _, label := range []string{"保存设置", "取消", tc.message} {
				rect, ok := tester.Find(label)
				if !ok || rect.Y < 0 || rect.Y+rect.H > 420 {
					t.Fatalf("update result pushed %q outside the minimum window: %+v", label, rect)
				}
			}
		})
	}
}

func TestUpdateWaitingStateKeepsSettingsAndDisablesDuplicateCheck(t *testing.T) {
	a := testApp()
	a.version, a.updateChecking = "0.0.5", true
	tester := ui.NewTester(a.view, 760, 420)
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("正在检查更新…"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tester.Frame()
	}
	if !a.settingsDialog || !a.updateChecking || !tester.HasText("正在检查更新…") {
		t.Fatal("waiting state closed settings or allowed a duplicate request")
	}
}

func TestUpdateResultFitsAlongsideSystemSettingsError(t *testing.T) {
	a := testApp()
	a.version = "0.0.5"
	a.integration = &fakeIntegration{stateError: errors.New("系统设置读取失败")}
	a.chooseSettings()
	a.updateMessage, a.updateFailed = locales.UpdateCheckFailed, true
	tester := ui.NewTester(a.view, 760, 420)
	for _, label := range []string{"保存设置", "取消", "重试", a.updateMessage.String()} {
		rect, ok := tester.Find(label)
		if !ok || rect.Y < 0 || rect.Y+rect.H > 420 {
			t.Fatalf("%q outside minimum window when both operations fail: %+v", label, rect)
		}
	}
}

func TestCanceledUpdateCheckClearsWaitingStateWithoutError(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	a.version = "0.0.5"
	a.chooseSettings()
	a.updateClient = &fakeUpdateClient{check: func(ctx context.Context, _ string) (*update.Release, error) {
		a.updateCancel()
		return nil, ctx.Err()
	}}
	a.checkUpdates(true)
	if !a.settingsDialog || a.updateChecking || a.updateFailed || a.updateMessage != nil {
		t.Fatal("canceled check closed settings or left a waiting/error state")
	}
}

func TestUpdateSettingsPreview(t *testing.T) {
	directory := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if directory == "" {
		t.Skip("set TEXTVIEWER_PREVIEW_DIR for update previews")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"浅色", "深色"} {
		for _, state := range []string{"waiting", "latest", "failed", "available"} {
			a := testApp()
			a.version, a.settings.Theme = "0.0.5", theme
			a.chooseSettings()
			switch state {
			case "waiting":
				a.updateChecking = true
			case "latest":
				a.updateMessage = locales.Text("当前已是最新版本 v0.0.5")
			case "failed":
				a.updateMessage, a.updateFailed = locales.UpdateCheckFailed, true
			case "available":
				a.availableUpdate = &update.Release{Version: "0.0.6"}
			}
			tester := ui.NewTester(a.view, 760, 420)
			file, err := os.Create(filepath.Join(directory, "update-"+state+"-"+theme+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, tester.Image())
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
