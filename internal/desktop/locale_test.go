package desktop

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/Oranjekop/TextViewer/internal/document"
	"github.com/Oranjekop/TextViewer/internal/search"
	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestLocaleSettingsSaveCancelAndRestart(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	if prefs := loadPreferences(); prefs.Locale != "" {
		t.Fatalf("new installation does not follow the system: %+v", prefs)
	}
	a := testApp()
	var applied []string
	a.setLocale = func(tag string) {
		applied = append(applied, tag)
	}
	tester := ui.NewTester(a.view, 760, 600)
	if err := tester.Click("设置"); err != nil {
		t.Fatal(err)
	}
	// The row label is also visible text. Click the actual adjacent trigger.
	r, _ := tester.Find("界面语言")
	tester.ClickAt(r.X+96+12+110, r.Y+r.H/2)
	if err := tester.ChooseMenuItem("English"); err != nil {
		t.Fatal(err)
	}
	if a.settingsDraft.Locale != "en" || a.settings.Locale != "" || !tester.HasText("设置") {
		t.Fatal("draft changed the active language before saving")
	}
	if err := tester.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 || loadPreferences().Locale != "" {
		t.Fatal("cancel persisted the draft or changed the active language")
	}
	a.chooseSettings()
	a.settingsDraft.Locale = "en"
	a.saveSettings(false)
	if !slices.Equal(applied, []string{"en"}) || a.settingsDialog || New().settings.Locale != "en" {
		t.Fatal("saved language did not apply or survive restart")
	}
	a.chooseSettings()
	a.settingsDraft.Locale = ""
	a.saveSettings(false)
	if !slices.Equal(applied, []string{"en", ""}) || loadPreferences().Locale != "" {
		t.Fatal("system language did not clear the explicit override")
	}
}

func TestLocaleSettingsReadFailurePreventsSave(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := testApp()
	applied := false
	a.setLocale = func(string) { applied = true }
	a.integration.(*fakeIntegration).stateError = errors.New("unavailable")
	a.chooseSettings()
	a.settingsDraft.Locale = "en"
	a.saveSettings(false)
	if applied || a.settings.Locale != "" || a.settingsError == nil || !a.settingsDialog {
		t.Fatal("failed settings operation applied the language draft")
	}
}

func TestLocalePreferencesNormalizeAndKeepOldSettings(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""}, {"zh-CN", "zh-Hans"}, {"zh-TW", "zh-Hans"}, {"en-US", "en"},
		{"en-GB", "en"}, {"de-DE", ""}, {"invalid_value", ""},
	} {
		if got := normalizeLocale(tc.input); got != tc.want {
			t.Errorf("%q normalized to %q, want %q", tc.input, got, tc.want)
		}
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := preferences{FontFamily: "Consolas", FontSize: 20, Wrap: false, Theme: "深色", Locale: "en-US"}
	if err := savePreferences(p); err != nil {
		t.Fatal(err)
	}
	got := loadPreferences()
	if got.Locale != "en" || got.FontFamily != p.FontFamily || got.FontSize != 20 || got.Theme != p.Theme || got.Wrap {
		t.Fatalf("locale normalization changed existing preferences: %+v", got)
	}
	// These are the same supported tags passed to MyGo by Ready. Matching and
	// system notifications belong to MyGo rather than a second OS-language probe.
	for _, tc := range []struct {
		preferred []string
		want      string
	}{
		{[]string{"zh-CN"}, "zh-Hans"}, {[]string{"en-GB"}, "en"},
		{[]string{"fr-FR", "zh-SG"}, "zh-Hans"}, {[]string{"fr-FR"}, ""},
	} {
		if got := i18n.Match(tc.preferred, []string{"en", "zh-Hans"}); got != tc.want {
			t.Errorf("system languages %v matched %q, want %q", tc.preferred, got, tc.want)
		}
	}
}

func TestLocaleStartupHonorsSavedSystemAndOneRunChoices(t *testing.T) {
	for _, tc := range []struct {
		saved, environment string
		apply              bool
	}{
		{"", "", true}, {"en", "", true}, {"zh-Hans", "", true}, {"zh-Hans", "en", false},
	} {
		t.Setenv("MYGO_LOCALE", tc.environment)
		a := testApp()
		a.settings.Locale = tc.saved
		applied := false
		a.setLocale = func(tag string) {
			applied = true
			if tag != tc.saved {
				t.Fatalf("startup ignored saved choice: got %q, want %q", tag, tc.saved)
			}
		}
		a.applyStartupLocale()
		if applied != tc.apply {
			t.Fatalf("saved=%q environment=%q: applied=%v, want %v", tc.saved, tc.environment, applied, tc.apply)
		}
	}
}

func TestLocaleChangesPreserveAllDocumentsAndTranslateMessages(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "zh-Hans")
	a := testApp()
	a.doc.Buffer.Edit(editor.Edit{Text: "first\nsecond"})
	a.state.Select(1, 3)
	a.setLanguage("JSON")
	a.addDocument(document.New())
	originalTabs := slices.Clone(a.tabs)
	before := a.tabs[0].doc.CaptureSave()
	state := a.tabs[0].state
	a.searchStatus = func() string { return locales.NotFound("needle") }
	a.status = locales.SettingsSaved
	a.updateMessage = func() string { return locales.UpToDate("0.0.9") }
	a.busy = locales.Finding
	_, _, err := search.Next(context.Background(), a.doc.Buffer.Snapshot(), search.Query{Text: "[", Regex: true}, 0, false)
	a.settingsError = err.Error
	a.chooseFont()
	t.Setenv("MYGO_LOCALE", "en")
	a.refreshLocale()
	if a.title != "" {
		t.Fatal("refreshing labels before window creation cached a title that the new window would never receive")
	}
	for _, text := range []string{a.searchStatus.String(), a.status.String(), a.updateMessage.String(), a.busy.String(), a.settingsError.String()} {
		if strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Fatalf("old-language transient text remained: %q", text)
		}
	}
	if a.tabs[0].doc.CaptureSave() != before || a.tabs[0].state != state ||
		!slices.Equal(a.tabs, originalTabs) || !a.tabs[0].doc.Buffer.CanUndo() ||
		a.tabs[0].language != "JSON" || state.Cursor() != 3 || a.fontInput != "Automatic" {
		t.Fatal("changing the UI language altered document state or font selection")
	}
	if a.tabs[0].name() != "Untitled.txt" || a.tabs[1].name() != "Untitled 2.txt" || state.Label != "Text editor" {
		t.Fatal("stored editor labels or untitled tab names did not change language")
	}
}

func TestLocaleEditorMenusAreConsistent(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "en"} {
		t.Run(tag, func(t *testing.T) {
			t.Setenv("MYGO_LOCALE", tag)
			a := testApp()
			a.doc.Path = "example.go"
			a.doc.Buffer.Set("package main\n")
			a.resetEditor()
			tester := ui.NewTester(a.view, 1120, 700)
			tester.SetLocale(tag)
			if err := tester.RightClick(locales.EditorArea()); err != nil {
				t.Fatal(err)
			}
			want := []string{"Cut", "Copy", "Paste", "Select All", "Toggle Comment", "Selection", "Transform", "Undo", "Redo", "Find", "Replace", "Format JSON"}
			selection, transform, expand, upper := "Selection", "Transform", "Expand Selection", "Upper Case"
			if tag == "zh-Hans" {
				want = []string{"剪切", "复制", "粘贴", "全选", "切换注释", "选择", "转换", "撤销", "重做", "查找", "替换", "格式化 JSON"}
				selection, transform, expand, upper = "选择", "转换", "扩大选择范围", "转为大写"
			}
			for _, label := range want {
				if !slices.Contains(tester.Menu(), label) {
					t.Fatalf("%s missing from %v", label, tester.Menu())
				}
			}
			if err := tester.ChooseMenuItem(selection, expand); err != nil {
				t.Fatal(err)
			}
			if err := tester.RightClick(locales.EditorArea()); err != nil {
				t.Fatal(err)
			}
			if err := tester.ChooseMenuItem(transform, upper); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(a.doc.Buffer.String(), "PACKAGE") {
				t.Fatal("translated submenu did not execute the existing editor action")
			}
		})
	}
}

func TestLocaleDialogsAndErrors(t *testing.T) {
	t.Setenv("MYGO_LOCALE", "en")
	a := testApp()
	a.doc.Buffer.Edit(editor.Edit{Text: "changed"})
	dialogs := a.dialogs.(*fakeDialogs)
	var seen mygo.MessageOptions
	dialogs.message = func(options mygo.MessageOptions) (mygo.MessageResult, error) {
		seen = options
		return mygo.MessageResult{Button: 2}, nil
	}
	a.confirmLeave(func() { t.Fatal("cancel left the document") })
	if seen.Message != "Save changes to “Untitled.txt”?" || !slices.Equal(seen.Buttons, []string{"Save", "Don't Save", "Cancel"}) {
		t.Fatalf("untranslated save dialog: %+v", seen)
	}
	if options := a.saveDialogOptions("example.txt", true); options.Title != "Save As" || options.Filters[0].Name != "TXT files" {
		t.Fatalf("untranslated save-as dialog: %+v", options)
	}
	for _, query := range []search.Query{{Text: "[", Regex: true}, {Text: `\q`, Escapes: true}} {
		_, _, err := search.Next(context.Background(), a.doc.Buffer.Snapshot(), query, 0, false)
		if err == nil || strings.ContainsFunc(err.Error(), func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Fatalf("untranslated search error: %v", err)
		}
	}
}

func TestLocaleLayoutAndPreview(t *testing.T) {
	output := os.Getenv("TEXTVIEWER_PREVIEW_DIR")
	if output != "" {
		if err := os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tag := range []string{"zh-Hans", "en"} {
		for _, theme := range []string{"浅色", "深色"} {
			t.Setenv("MYGO_LOCALE", tag)
			a := testApp()
			a.version, a.settings.Theme = "0.0.9", theme
			tester := ui.NewTester(a.view, 760, 420)
			tester.SetLocale(tag)
			for _, scale := range []float32{1, 1.5, 2} {
				tester.SetScale(scale)
				a.chooseSettings()
				a.settingsError = locales.UpdateCheckFailed
				a.settingsReadFailed = true
				a.updateMessage = func() string { return locales.UpToDate("0.0.9") }
				tester.Frame()
				labels := []string{locales.Appearance(), locales.InterfaceLanguage(), locales.FileAssociations(), contextMenuLabel(), locales.Retry(), locales.CheckUpdates(), locales.Cancel(), locales.SaveSettings()}
				if runtime.GOOS != "darwin" {
					labels = append(labels, locales.DefaultApps())
				}
				for _, label := range labels {
					rect, ok := tester.Find(label)
					if !ok || rect.W == 0 || rect.H == 0 || rect.X < 0 || rect.Y < 0 || rect.X+rect.W > 760 || rect.Y+rect.H > 420 {
						t.Fatalf("%s %s scale %g: %q outside minimum window: %+v", tag, theme, scale, label, rect)
					}
				}
				writeLocalePreview(t, output, fmt.Sprintf("settings-%s-%s-%g", tag, theme, scale), tester)
				a.settingsDialog = false
				a.findOpen, a.replacing = true, true
				tester.Frame()
				var previous ui.Rect
				for i, label := range []string{locales.QueryLabel(), locales.MatchCase(), locales.Regex(), locales.Escapes(), locales.Previous(), locales.Next(), locales.CloseFind()} {
					rect, ok := tester.Find(label)
					if !ok || rect.W == 0 || rect.X < 0 || rect.X+rect.W > 760 || i > 0 && previous.X+previous.W > rect.X+0.5 {
						t.Fatalf("%s scale %g: overlapping or clipped search control %q: %+v, previous %+v", tag, scale, label, rect, previous)
					}
					previous = rect
				}
				writeLocalePreview(t, output, fmt.Sprintf("find-%s-%s-%g", tag, theme, scale), tester)
			}
		}
	}
}

func writeLocalePreview(t *testing.T, directory, name string, tester *ui.Tester) {
	t.Helper()
	if directory == "" {
		return
	}
	f, err := os.Create(filepath.Join(directory, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tester.Image()); err != nil {
		t.Fatal(err)
	}
}
