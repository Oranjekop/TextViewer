package fileassoc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"github.com/Oranjekop/TextViewer/internal/shellmenu"
	"github.com/Oranjekop/TextViewer/locales"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	classesPath      = `Software\Classes\`
	capabilitiesPath = `Software\Oranjekop\TextViewer\Capabilities`
	statePath        = `Software\Oranjekop\TextViewer\FileAssociations`
	registeredPath   = `Software\RegisteredApplications`
	ownerValue       = "TextViewerDirectory"
)

var (
	shell32        = windows.NewLazySystemDLL("shell32.dll")
	shellChange    = shell32.NewProc("SHChangeNotify")
	validExtension = regexp.MustCompile(`^\.[a-z0-9]+$`)
)

func Register() error {
	dir, err := shellmenu.PrepareBundle()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := register(registry.CURRENT_USER, dir, filepath.Dir(exe)); err != nil {
		return err
	}
	notifyShell()
	return nil
}

func Unregister() error {
	state, err := readState(registry.CURRENT_USER)
	if err != nil || state.Directory == "" {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	source := filepath.Dir(exe)
	if !sameDirectory(source, state.Directory) && !sameDirectory(source, state.Source) {
		dir, err := shellmenu.BundleDirectory()
		if err != nil {
			return err
		}
		if !sameDirectory(dir, state.Directory) {
			return nil // An older copy must not remove a newer registration.
		}
	}
	if err := unregister(registry.CURRENT_USER, state.Directory); err != nil {
		return err
	}
	notifyShell()
	return nil
}

func Registered() (bool, error) {
	state, err := readState(registry.CURRENT_USER)
	return state.Directory != "", err
}

// RemoveRegistration is an explicit user opt-out, including a registration
// owned by another portable copy. Installer uninstall retains its owner guard.
func RemoveRegistration() error {
	state, err := readState(registry.CURRENT_USER)
	if err != nil || state.Directory == "" {
		return err
	}
	if err := unregister(registry.CURRENT_USER, state.Directory); err != nil {
		return err
	}
	notifyShell()
	return nil
}

func OpenDefaults() error {
	uri, _ := windows.UTF16PtrFromString("ms-settings:defaultapps?registeredAppUser=TextViewer")
	return windows.ShellExecute(0, nil, uri, nil, nil, windows.SW_SHOWNORMAL)
}

type registrationState struct {
	Directory, Source string
	Extensions        []string
}

func readState(root registry.Key) (registrationState, error) {
	var state registrationState
	key, err := registry.OpenKey(root, statePath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer key.Close()
	state.Directory, _, err = key.GetStringValue("Directory")
	if err != nil {
		return state, err
	}
	state.Source, _, err = key.GetStringValue("Source")
	if err != nil {
		return state, err
	}
	state.Extensions, _, err = key.GetStringsValue("Extensions")
	if err != nil {
		return state, err
	}
	for _, ext := range state.Extensions {
		if !validExtension.MatchString(ext) {
			return state, fmt.Errorf("invalid stored file extension %q", ext)
		}
	}
	return state, nil
}

func register(root registry.Key, dir, source string) error {
	if !filepath.IsAbs(dir) || !filepath.IsAbs(source) || strings.ContainsAny(dir, "\"\r\n") {
		return locales.NewError(locales.InvalidProgramPath)
	}
	exe := filepath.Join(dir, "TextViewer.exe")
	if info, err := os.Stat(exe); err != nil || !info.Mode().IsRegular() {
		return locales.NewError(func() string { return locales.ProgramNotFound(exe) })
	}
	if err := writeIcons(dir); err != nil {
		return err
	}
	old, err := readState(root)
	if err != nil {
		return err
	}
	command := openCommand(dir)
	applicationIcon := `"` + exe + `",0`
	var extensions []string
	for _, group := range filetypes.AssociationGroups() {
		for _, ext := range group.Extensions {
			extensions = append(extensions, ext)
			progID := filetypes.ProgID(ext)
			base := classesPath + progID
			if err := writeStrings(root, base, map[string]string{
				"":         strings.ToUpper(ext[1:]) + " " + group.Name + "文件",
				ownerValue: dir,
			}); err != nil {
				return err
			}
			for path, values := range map[string]map[string]string{
				base + `\DefaultIcon`:        {"": `"` + filepath.Join(dir, group.IconName()) + `",0`},
				base + `\shell\open`:         {"MultiSelectModel": "Single"},
				base + `\shell\open\command`: {"": command},
				base + `\Application`: {
					"ApplicationName":        "TextViewer",
					"ApplicationDescription": "TextViewer 文本编辑器",
					"ApplicationIcon":        applicationIcon,
				},
			} {
				if err := writeStrings(root, path, values); err != nil {
					return err
				}
			}
			key, _, err := registry.CreateKey(root, classesPath+ext+`\OpenWithProgids`, registry.SET_VALUE)
			if err != nil {
				return err
			}
			err = key.SetStringValue(progID, "")
			key.Close()
			if err != nil {
				return err
			}
			if err := writeStrings(root, capabilitiesPath+`\FileAssociations`, map[string]string{ext: progID}); err != nil {
				return err
			}
		}
	}
	if err := writeStrings(root, capabilitiesPath, map[string]string{
		"ApplicationName": "TextViewer", "ApplicationDescription": "TextViewer 文本编辑器",
		"ApplicationIcon": applicationIcon, ownerValue: dir,
	}); err != nil {
		return err
	}
	if err := writeStrings(root, registeredPath, map[string]string{"TextViewer": capabilitiesPath}); err != nil {
		return err
	}
	// An upgraded catalog can retire a format without retaining stale handlers.
	for _, ext := range old.Extensions {
		if !slices.Contains(extensions, ext) {
			if err := removeFormat(root, old.Directory, ext); err != nil {
				return err
			}
			if err := deleteValue(root, capabilitiesPath+`\FileAssociations`, ext); err != nil {
				return err
			}
		}
	}
	// Preserve the old inventory until retirement succeeds so it can be retried.
	if err := writeStrings(root, statePath, map[string]string{"Directory": dir, "Source": source}); err != nil {
		return err
	}
	key, err := registry.OpenKey(root, statePath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringsValue("Extensions", extensions)
}

func unregister(root registry.Key, expectedDir string) error {
	state, err := readState(root)
	if err != nil || state.Directory == "" || !sameDirectory(state.Directory, expectedDir) {
		return err
	}
	for _, ext := range state.Extensions {
		if err := removeFormat(root, state.Directory, ext); err != nil {
			return err
		}
	}
	owner, err := readStringValue(root, capabilitiesPath, ownerValue)
	if err != nil {
		return err
	}
	if sameDirectory(owner, state.Directory) {
		if err := deleteTree(root, capabilitiesPath); err != nil {
			return err
		}
		registered, err := readStringValue(root, registeredPath, "TextViewer")
		if err != nil {
			return err
		}
		if registered == capabilitiesPath {
			if err := deleteValue(root, registeredPath, "TextViewer"); err != nil {
				return err
			}
		}
	}
	return deleteTree(root, statePath)
}

func removeFormat(root registry.Key, dir, ext string) error {
	base := classesPath + filetypes.ProgID(ext)
	owner, err := readStringValue(root, base, ownerValue)
	if err != nil {
		return err
	}
	if !sameDirectory(owner, dir) {
		return nil
	}
	if err := deleteValue(root, classesPath+ext+`\OpenWithProgids`, filetypes.ProgID(ext)); err != nil {
		return err
	}
	return deleteTree(root, base)
}

func openCommand(dir string) string {
	return `"` + filepath.Join(dir, "TextViewer.exe") + `" -- "%1"`
}

func sameDirectory(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func writeStrings(root registry.Key, path string, values map[string]string) error {
	key, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	for name, value := range values {
		if err := key.SetStringValue(name, value); err != nil {
			return err
		}
	}
	return nil
}

func readStringValue(root registry.Key, path, name string) (string, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return value, err
}

func deleteValue(root registry.Key, path, name string) error {
	key, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	err = key.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func deleteTree(root registry.Key, path string) error {
	key, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := key.ReadSubKeyNames(-1)
	key.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := deleteTree(root, path+`\`+name); err != nil {
			return err
		}
	}
	return registry.DeleteKey(root, path)
}

func notifyShell() {
	// FLUSHNOWAIT refreshes associations without waiting for Explorer to redraw.
	shellChange.Call(0x08000000, 0x2000, 0, 0)
}
