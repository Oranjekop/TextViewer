package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui"
)

func TestAssociationChooserDraftCancelAndSave(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("per-extension registrations use Windows settings")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	a.settings.AssociationExtensions = []string{".txt", ".go"}
	backend := &fakeIntegration{state: integrationState{Associations: true}}
	a.integration = backend
	tester := ui.NewTester(a.view, 760, 420)
	for _, label := range []string{locales.Settings(), locales.ChooseExtensions()} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if !a.associationDialog || !a.associationDraft[".go"] || a.associationDraft[".bat"] {
		t.Fatal("selection did not restore or offered command scripts")
	}
	if err := tester.Click(".txt"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.settingsDraft.extensions, []string{".txt", ".go"}) || len(backend.calls) > 0 {
		t.Fatal("cancel changed the saved selection or system registrations")
	}
	if err := tester.Click(locales.ChooseExtensions()); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click(".txt"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click(locales.Apply()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.settingsDraft.extensions, []string{".go"}) || len(backend.calls) > 0 {
		t.Fatal("applying the chooser should change only the settings draft")
	}
	if err := tester.Click(locales.SaveSettings()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backend.extensions, []string{".go"}) || !slices.Equal(loadPreferences().AssociationExtensions, []string{".go"}) {
		t.Fatal("changing suffixes while enabled was not registered and persisted")
	}
	if !reflect.DeepEqual(backend.calls, []string{"associations=true", "cleanup"}) {
		t.Fatalf("unexpected integration work: %v", backend.calls)
	}
	backend.calls = nil
	a.chooseSettings()
	a.saveSettings(false)
	if len(backend.calls) != 0 {
		t.Fatal("unchanged settings reapplied associations")
	}
	if err := configureInstalledIntegration(backend, loadPreferences()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(backend.extensions, []string{".go"}) {
		t.Fatal("upgrade lost the selected suffixes")
	}
}

func TestAssociationChooserLayoutAndPreview(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the extension chooser is a Windows settings control")
	}
	for _, locale := range []string{"zh-Hans", "en"} {
		for _, theme := range []string{"浅色", "深色"} {
			t.Setenv("MYGO_LOCALE", locale)
			a := testApp()
			a.settings.Theme = theme
			a.chooseSettings()
			tester := ui.NewTester(a.view, 760, 420)
			tester.SetLocale(locale)
			tester.Frame()
			msgs := locales.In(locale)
			for _, label := range []string{msgs.ChooseExtensions(), msgs.DefaultApps()} {
				r, ok := tester.Find(label)
				if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 760 || r.Y+r.H > 420 {
					t.Fatalf("settings control %q outside minimum window: %+v", label, r)
				}
			}
			a.chooseAssociations()
			tester.Frame()
			for _, label := range []string{msgs.ChooseExtensions(), msgs.DefaultExtensions(), msgs.Cancel(), msgs.Apply()} {
				r, ok := tester.Find(label)
				if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 760 || r.Y+r.H > 420 {
					t.Fatalf("chooser control %q outside minimum window: %+v", label, r)
				}
			}
			if directory := os.Getenv("TEXTVIEWER_PREVIEW_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(directory, "associations-"+locale+"-"+theme+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tester.Image())
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestFailedMenuSaveRestoresPreviousSelectedExtensions(t *testing.T) {
	backend := &fakeIntegration{state: integrationState{Associations: true}, menuError: os.ErrPermission}
	err := applyIntegrations(backend, integrationState{Associations: true, ContextMenu: true}, backend.state, integrationOptions{
		selectionChanged: true, extensions: []string{".go"}, previousExtensions: []string{".txt"},
	})
	if err == nil || !slices.Equal(backend.extensions, []string{".txt"}) {
		t.Fatalf("failed menu save did not restore suffix selection: %v %v", backend.extensions, err)
	}
}
