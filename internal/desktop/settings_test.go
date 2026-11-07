package desktop

import (
	"context"
	"reflect"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/update"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

func TestSettingsReopenKeepsSavedIntegrationChoices(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			t.Setenv("APPDATA", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			prefs := preferences{
				FontFamily: "Consolas", FontSize: 20, Wrap: false,
				Theme: "深色", Locale: "en", AutoCheckUpdates: false,
				FileAssociations: &enabled, ContextMenu: &enabled,
			}
			if err := savePreferences(prefs); err != nil {
				t.Fatal(err)
			}
			a := New()
			backend := &fakeIntegration{state: integrationState{!enabled, !enabled}}
			a.integration = backend
			a.chooseSettings()
			if a.settingsDraft.Associations != enabled || a.settingsDraft.ContextMenu != enabled {
				t.Fatalf("system state replaced saved choices: saved=%t draft=%+v", enabled, a.settingsDraft)
			}
			if a.settingsDraft.initialIntegration != backend.state || backend.reads != 1 || len(backend.calls) != 0 {
				t.Fatal("opening settings changed Windows integration or lost its baseline")
			}
			a.saveSettings(false)
			want := []string{"associations=false", "menu=false", "cleanup"}
			if enabled {
				want = []string{"associations=true", "menu=true", "cleanup"}
			}
			if !reflect.DeepEqual(backend.calls, want) || backend.state != (integrationState{enabled, enabled}) {
				t.Fatalf("save did not apply the remembered choices: state=%+v calls=%v", backend.state, backend.calls)
			}
			if a.settingsError != nil || !reflect.DeepEqual(New().settings, prefs) {
				t.Fatalf("save changed remembered preferences: %+v err=%v", New().settings, a.settingsError)
			}
			backend.calls = nil
			a.chooseSettings()
			a.saveSettings(false)
			if len(backend.calls) != 0 {
				t.Fatalf("saving unchanged settings repeated Windows operations: %v", backend.calls)
			}
		})
	}
}

func TestSettingsUnsavedIntegrationChoiceUsesSystemState(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	disabled := false
	prefs := preferences{
		FontSize: 16, Wrap: true, Theme: "浅色",
		FileAssociations: &disabled,
	}
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	a := New()
	backend := &fakeIntegration{state: integrationState{true, true}}
	a.integration = backend
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click(locales.Settings()); err != nil {
		t.Fatal(err)
	}
	if a.settingsDraft.Associations || !a.settingsDraft.ContextMenu {
		t.Fatalf("saved and unspecified choices were not restored independently: %+v", a.settingsDraft)
	}
	if err := tester.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	if len(backend.calls) != 0 || !reflect.DeepEqual(New().settings, prefs) {
		t.Fatal("cancel changed remembered preferences or Windows integration")
	}
}

func TestSettingsSurviveFontWrapAndManualUpdateCheck(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	enabled := true
	prefs := preferences{
		FontFamily: "Consolas", FontSize: 20, Wrap: false,
		Theme: "深色", Locale: "en", AutoCheckUpdates: false,
		FileAssociations: &enabled, ContextMenu: &enabled,
	}
	if err := savePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.integration = &fakeIntegration{state: integrationState{true, true}}
	a.zoom(2)
	a.wrap = true
	a.persistPreferences()
	a.version = "0.0.17"
	a.updateClient = &fakeUpdateClient{check: func(context.Context, string) (*update.Release, error) {
		return nil, nil
	}}
	a.checkUpdates(true)
	restarted := New()
	got := restarted.settings
	prefs.FontSize, prefs.Wrap = 22, true
	if !reflect.DeepEqual(got, prefs) {
		t.Fatalf("an unrelated save replaced existing preferences: %+v", got)
	}
	restarted.integration = a.integration
	restarted.chooseSettings()
	if draft := restarted.settingsDraft; draft.Theme != "深色" || draft.Locale != "en" ||
		draft.AutoUpdate || !draft.Associations || !draft.ContextMenu ||
		restarted.fontSize != "22" || !restarted.wrap || restarted.editorFont() != "Consolas" {
		t.Fatalf("restart did not restore the saved controls: prefs=%+v draft=%+v", got, draft)
	}
}

func chooseSettingsMenu(t *testing.T, tester *ui.Tester, label, choice string) {
	t.Helper()
	row, ok := tester.Find(label)
	if !ok {
		t.Fatalf("settings row %q not found", label)
	}
	tester.ClickAt(row.X+96+12+110, row.Y+row.H/2)
	if err := tester.ChooseMenuItem(choice); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsSelectionsSurviveSaveReopenAndRestart(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	a := New()
	a.integration = &fakeIntegration{}
	tester := ui.NewTester(a.view, 1120, 700)
	if err := tester.Click(locales.Settings()); err != nil {
		t.Fatal(err)
	}
	chooseSettingsMenu(t, tester, locales.Appearance(), locales.Dark())
	chooseSettingsMenu(t, tester, locales.InterfaceLanguage(), "English")
	for _, label := range []string{locales.AutoUpdate(), locales.FileAssociations(), contextMenuLabel()} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.settingsDraft.Theme != "深色" || a.settingsDraft.Locale != "en" ||
		a.settingsDraft.AutoUpdate || !a.settingsDraft.Associations || !a.settingsDraft.ContextMenu {
		t.Fatalf("UI choices did not reach the draft: %+v", a.settingsDraft)
	}
	if err := tester.Click(locales.SaveSettings()); err != nil {
		t.Fatal(err)
	}
	if a.settingsDialog || a.settingsError != nil {
		t.Fatalf("settings did not save: open=%v err=%v", a.settingsDialog, a.settingsError)
	}
	for _, app := range []*App{a, New()} {
		if app != a {
			app.integration = a.integration
			tester = ui.NewTester(app.view, 1120, 700)
		}
		if err := tester.Click(locales.Settings()); err != nil {
			t.Fatal(err)
		}
		draft := app.settingsDraft
		if draft.Theme != "深色" || draft.Locale != "en" || draft.AutoUpdate ||
			!draft.Associations || !draft.ContextMenu {
			t.Fatalf("saved choices were lost after reopening: %+v", draft)
		}
	}
}
