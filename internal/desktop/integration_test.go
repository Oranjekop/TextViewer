package desktop

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

type fakeIntegration struct {
	state      integrationState
	calls      []string
	reads      int
	stateError error
	menuError  error
}

func (f *fakeIntegration) State() (integrationState, error) {
	f.reads++
	return f.state, f.stateError
}
func (f *fakeIntegration) Associations(enabled bool) error {
	f.calls = append(f.calls, fmt.Sprintf("associations=%t", enabled))
	f.state.Associations = enabled
	return nil
}
func (f *fakeIntegration) ContextMenu(enabled, trust bool) error {
	f.calls = append(f.calls, fmt.Sprintf("menu=%t", enabled))
	if f.menuError != nil {
		return f.menuError
	}
	f.state.ContextMenu = enabled
	return nil
}
func (f *fakeIntegration) Cleanup() error {
	f.calls = append(f.calls, "cleanup")
	return nil
}

func TestSettingsCheckboxesApplyOnlyOnSaveAndPersistOptOut(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	backend := &fakeIntegration{state: integrationState{true, true}}
	a.integration = backend
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsDraft.Associations || !a.settingsDraft.ContextMenu {
		t.Fatal("checkboxes did not reflect the registered state")
	}
	for _, name := range []string{"关联常用文件格式", "Windows 11 右键菜单", "取消"} {
		if err := tester.Click(name); err != nil {
			t.Fatal(err)
		}
	}
	if len(backend.calls) != 0 || a.settings.FileAssociations != nil {
		t.Fatal("cancel changed integration state or preferences")
	}
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"关联常用文件格式", "Windows 11 右键菜单", "保存设置"} {
		if err := tester.Click(name); err != nil {
			t.Fatal(err)
		}
	}
	if backend.state != (integrationState{}) || a.settingsDialog {
		t.Fatal("saving unchecked options did not remove both integrations")
	}
	p := loadPreferences()
	if p.FileAssociations == nil || *p.FileAssociations || p.ContextMenu == nil || *p.ContextMenu {
		t.Fatal("integration opt-out was not persisted")
	}
	backend.calls = nil
	// Even if old entries reappear, an upgrade honors the saved opt-out.
	backend.state = integrationState{true, true}
	if err := configureInstalledIntegration(backend, p); err != nil {
		t.Fatal(err)
	}
	if want := []string{"associations=false", "menu=false", "cleanup"}; !reflect.DeepEqual(backend.calls, want) {
		t.Fatalf("installer did not honor opt-out: %v", backend.calls)
	}
}

func TestSettingsEnableAndInstallerPreservesExistingChoices(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current integrationState
		want    []string
	}{
		{"fresh install", integrationState{}, []string{"cleanup"}},
		{"upgrade opted in", integrationState{true, true}, []string{"associations=true", "menu=true", "cleanup"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &fakeIntegration{state: tc.current}
			if err := configureInstalledIntegration(backend, preferences{}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(backend.calls, tc.want) {
				t.Fatalf("calls %v, want %v", backend.calls, tc.want)
			}
		})
	}
	backend := &fakeIntegration{}
	if err := applyIntegrations(backend, integrationState{true, true}, integrationState{}, false); err != nil {
		t.Fatal(err)
	}
	if backend.state != (integrationState{true, true}) {
		t.Fatal("checked options were not enabled")
	}
}

func TestSettingsOptionsSaveIndependently(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uncheck string
		want    integrationState
		calls   []string
	}{
		{"remove associations", "关联常用文件格式", integrationState{false, true}, []string{"associations=false", "menu=true", "cleanup"}},
		{"remove menu", "Windows 11 右键菜单", integrationState{true, false}, []string{"associations=true", "menu=false", "cleanup"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APPDATA", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			a := testApp()
			backend := &fakeIntegration{state: integrationState{true, true}}
			a.integration = backend
			tester := ui.NewTester(a.view, 760, 600)
			for _, name := range []string{"设置", tc.uncheck, "保存设置"} {
				if err := tester.Click(name); err != nil {
					t.Fatal(err)
				}
			}
			if backend.state != tc.want || !reflect.DeepEqual(backend.calls, tc.calls) {
				t.Fatalf("saved state %v, calls %v", backend.state, backend.calls)
			}
			p := loadPreferences()
			if p.FileAssociations == nil || p.ContextMenu == nil ||
				*p.FileAssociations != tc.want.Associations || *p.ContextMenu != tc.want.ContextMenu {
				t.Fatal("independent checkbox choices were not persisted")
			}
		})
	}
}

func TestFailedMenuChangeRestoresAssociationsAndSkipsCleanup(t *testing.T) {
	rejected := errors.New("registration rejected")
	backend := &fakeIntegration{menuError: rejected}
	err := applyIntegrations(backend, integrationState{true, true}, integrationState{}, false)
	if !errors.Is(err, rejected) || backend.state.Associations {
		t.Fatalf("failed save left new file associations enabled: %v %v", backend.state, err)
	}
	if want := []string{"associations=true", "menu=true", "associations=false"}; !reflect.DeepEqual(backend.calls, want) {
		t.Fatalf("unexpected failure actions: %v", backend.calls)
	}
}

func TestSettingsStateIsNotQueriedDuringRedraw(t *testing.T) {
	a := testApp()
	backend := a.integration.(*fakeIntegration)
	tester := ui.NewTester(a.view, 760, 420)
	_ = tester.Click("设置")
	for range 5 {
		tester.Frame()
	}
	if backend.reads != 1 || len(backend.calls) != 0 {
		t.Fatal("redrawing settings performed integration work")
	}
	for _, name := range []string{"关联常用文件格式", "Windows 11 右键菜单", "保存设置"} {
		rect, ok := tester.Find(name)
		if !ok || rect.X < 0 || rect.Y < 0 || rect.X+rect.W > 760 || rect.Y+rect.H > 420 {
			t.Fatalf("settings control %s outside minimum window: %+v", name, rect)
		}
	}
}

func TestSettingsPreview(t *testing.T) {
	directory := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if directory == "" {
		t.Skip("set TEXTVIEWER_PREVIEW_DIR for settings previews")
	}
	for _, theme := range []string{"浅色", "深色"} {
		a := testApp()
		a.version, a.settings.Theme = "0.0.4", theme
		a.integration = &fakeIntegration{state: integrationState{true, true}}
		tester := ui.NewTester(a.view, 760, 420)
		_ = tester.Click("设置")
		path := filepath.Join(directory, "settings-"+theme+".png")
		file, err := os.Create(path)
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
