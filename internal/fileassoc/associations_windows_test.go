package fileassoc

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func isolatedRegistry(t *testing.T) registry.Key {
	t.Helper()
	path := `Software\TextViewer.FileAssociations.Tests.` + strings.ToLower(rand.Text())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		key.Close()
		if err := deleteTree(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	return key
}

func applicationFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "中文 application with spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TextViewer.exe"), []byte("test only; never execute"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustWrite(t *testing.T, root registry.Key, path string, values map[string]string) {
	t.Helper()
	if err := writeStrings(root, path, values); err != nil {
		t.Fatal(err)
	}
}

func assertString(t *testing.T, root registry.Key, path, name, want string) {
	t.Helper()
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(path, err)
	}
	defer key.Close()
	got, _, err := key.GetStringValue(name)
	if err != nil || got != want {
		t.Fatalf("%s\\%s = %q; want %q", path, name, got, want)
	}
}

func assertMissingKey(t *testing.T, root registry.Key, path string) {
	t.Helper()
	key, err := registry.OpenKey(root, path, registry.READ)
	if err == nil {
		key.Close()
		t.Fatalf("left registry key %s", path)
	}
	if err != registry.ErrNotExist {
		t.Fatal(err)
	}
}

func TestRegistrationAndRemovalPreserveDefaultsAndOtherHandlers(t *testing.T) {
	root, dir := isolatedRegistry(t), applicationFixture(t)
	fileExts := `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`
	for _, ext := range filetypes.Extensions() {
		ext = "." + ext
		mustWrite(t, root, classesPath+ext, map[string]string{"": "Other.Editor", "Content Type": "text/plain"})
		mustWrite(t, root, classesPath+ext+`\DefaultIcon`, map[string]string{"": `"C:\Other\Editor.exe",0`})
		mustWrite(t, root, classesPath+ext+`\OpenWithProgids`, map[string]string{"Other.Editor": ""})
		for _, choice := range []string{`UserChoice`, `UserChoiceLatest\ProgId`} {
			mustWrite(t, root, fileExts+ext+`\`+choice, map[string]string{"ProgId": "Other.Editor", "Hash": "unchanged hash"})
		}
	}
	mustWrite(t, root, registeredPath, map[string]string{"Other.Editor": `Software\Other\Capabilities`})
	checkPreserved := func() {
		t.Helper()
		assertString(t, root, registeredPath, "Other.Editor", `Software\Other\Capabilities`)
		for _, ext := range filetypes.Extensions() {
			ext = "." + ext
			assertString(t, root, classesPath+ext, "", "Other.Editor")
			assertString(t, root, classesPath+ext, "Content Type", "text/plain")
			assertString(t, root, classesPath+ext+`\DefaultIcon`, "", `"C:\Other\Editor.exe",0`)
			assertString(t, root, classesPath+ext+`\OpenWithProgids`, "Other.Editor", "")
			for _, choice := range []string{`UserChoice`, `UserChoiceLatest\ProgId`} {
				assertString(t, root, fileExts+ext+`\`+choice, "ProgId", "Other.Editor")
				assertString(t, root, fileExts+ext+`\`+choice, "Hash", "unchanged hash")
			}
		}
	}
	start := time.Now()
	if err := register(root, dir, dir); err != nil {
		t.Fatal(err)
	}
	t.Logf("Registered %d extensions in %s (registry and icon writes; excludes bundle preparation)", len(filetypes.Extensions()), time.Since(start))
	if err := register(root, dir, dir); err != nil {
		t.Fatal("repeated registration:", err)
	}
	checkPreserved()
	assertString(t, root, registeredPath, "TextViewer", capabilitiesPath)
	for _, group := range filetypes.AssociationGroups() {
		path := filepath.Join(dir, group.IconName())
		want, _ := icons.ReadFile("icons/" + group.IconName())
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s was not extracted intact: %v", group.ID, err)
		}
		for _, ext := range group.Extensions {
			base := classesPath + filetypes.ProgID(ext)
			assertString(t, root, base+`\shell\open\command`, "", openCommand(dir))
			assertString(t, root, base+`\DefaultIcon`, "", `"`+path+`",0`)
			assertString(t, root, base+`\Application`, "ApplicationName", "TextViewer")
			assertString(t, root, capabilitiesPath+`\FileAssociations`, ext, filetypes.ProgID(ext))
			handler, err := registry.OpenKey(root, classesPath+ext+`\OpenWithProgids`, registry.QUERY_VALUE)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = handler.GetStringValue(filetypes.ProgID(ext))
			handler.Close()
			if err != nil {
				t.Fatal("format missing from Open with:", ext, err)
			}
		}
	}
	for _, ext := range []string{".bat", ".cmd", ".ps1", ".sh", ".py", ".js", ".go"} {
		assertMissingKey(t, root, classesPath+filetypes.ProgID(ext))
	}
	document := `C:\中文 folder\-sample & document.txt`
	args, err := windows.DecomposeCommandLine(strings.ReplaceAll(openCommand(dir), "%1", document))
	if err != nil || !reflect.DeepEqual(args, []string{filepath.Join(dir, "TextViewer.exe"), "--", document}) {
		t.Fatalf("unsafe or ambiguous open command: %q %v", args, err)
	}
	start = time.Now()
	if err := unregister(root, dir); err != nil {
		t.Fatal(err)
	}
	t.Logf("Removed associations in %s", time.Since(start))
	if err := unregister(root, dir); err != nil {
		t.Fatal("repeated removal:", err)
	}
	assertMissingKey(t, root, capabilitiesPath)
	assertMissingKey(t, root, statePath)
	checkPreserved()
	for _, ext := range filetypes.Extensions() {
		ext = "." + ext
		assertMissingKey(t, root, classesPath+filetypes.ProgID(ext))
	}
}

func TestUpgradeMovesAllCommandsAndRetiresOnlyOwnedMappings(t *testing.T) {
	root, oldDir, newDir := isolatedRegistry(t), applicationFixture(t), applicationFixture(t)
	if err := register(root, oldDir, oldDir); err != nil {
		t.Fatal(err)
	}
	retired := []string{".bat", ".cmd", ".ps1", ".sh", ".py", ".js", ".go", ".retired"}
	for _, ext := range retired {
		mustWrite(t, root, classesPath+filetypes.ProgID(ext), map[string]string{ownerValue: oldDir})
		mustWrite(t, root, classesPath+ext+`\OpenWithProgids`, map[string]string{filetypes.ProgID(ext): "", "Other.Editor": ""})
		mustWrite(t, root, capabilitiesPath+`\FileAssociations`, map[string]string{ext: filetypes.ProgID(ext)})
		mustWrite(t, root, classesPath+ext, map[string]string{"": "Native.Handler"})
	}
	state, _ := readState(root)
	key, _ := registry.OpenKey(root, statePath, registry.SET_VALUE)
	if err := key.SetStringsValue("Extensions", append(state.Extensions, retired...)); err != nil {
		t.Fatal(err)
	}
	key.Close()
	if err := register(root, newDir, newDir); err != nil {
		t.Fatal(err)
	}
	for _, ext := range retired {
		assertMissingKey(t, root, classesPath+filetypes.ProgID(ext))
		assertString(t, root, classesPath+ext+`\OpenWithProgids`, "Other.Editor", "")
		assertString(t, root, classesPath+ext, "", "Native.Handler")
		key, err := registry.OpenKey(root, capabilitiesPath+`\FileAssociations`, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = key.GetStringValue(ext)
		key.Close()
		if err != registry.ErrNotExist {
			t.Fatalf("retired capability %s retained: %v", ext, err)
		}
	}
	for _, group := range filetypes.AssociationGroups() {
		for _, ext := range group.Extensions {
			assertString(t, root, classesPath+filetypes.ProgID(ext)+`\shell\open\command`, "", openCommand(newDir))
		}
	}
	if err := unregister(root, oldDir); err != nil {
		t.Fatal(err)
	}
	state, err := readState(root)
	if err != nil || state.Directory != newDir {
		t.Fatalf("an older copy removed the new registration: %+v %v", state, err)
	}
	mustWrite(t, root, registeredPath, map[string]string{"TextViewer": `Software\Custom\Capabilities`})
	if err := unregister(root, newDir); err != nil {
		t.Fatal(err)
	}
	assertString(t, root, registeredPath, "TextViewer", `Software\Custom\Capabilities`)
}

func TestSelectedDevelopmentAssociationsAndDeselection(t *testing.T) {
	root, dir := isolatedRegistry(t), applicationFixture(t)
	if err := register(root, dir, dir, []string{".txt", ".go", ".py", ".js"}); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".txt", ".go", ".py", ".js"} {
		assertString(t, root, classesPath+filetypes.ProgID(ext)+`\shell\open\command`, "", openCommand(dir))
	}
	for _, ext := range []string{".bat", ".cmd", ".ps1", ".sh", ".json"} {
		assertMissingKey(t, root, classesPath+filetypes.ProgID(ext))
	}
	mustWrite(t, root, classesPath+`.py\OpenWithProgids`, map[string]string{"Python.File": ""})
	mustWrite(t, root, classesPath+`.py`, map[string]string{"": "Python.File"})
	if err := register(root, dir, dir, []string{".txt"}); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".go", ".py", ".js"} {
		assertMissingKey(t, root, classesPath+filetypes.ProgID(ext))
	}
	assertString(t, root, classesPath+`.py`, "", "Python.File")
	assertString(t, root, classesPath+`.py\OpenWithProgids`, "Python.File", "")
	if err := register(root, dir, dir, []string{".bat"}); err == nil {
		t.Fatal("accepted executable command script")
	}
	state, err := readState(root)
	if err != nil || !reflect.DeepEqual(state.Extensions, []string{".txt"}) {
		t.Fatalf("invalid selection changed registry inventory: %+v %v", state, err)
	}
}

func TestUnregisterPreservesReassignedProgID(t *testing.T) {
	root, dir := isolatedRegistry(t), applicationFixture(t)
	if err := register(root, dir, dir); err != nil {
		t.Fatal(err)
	}
	base := classesPath + filetypes.ProgID(".txt")
	mustWrite(t, root, base, map[string]string{ownerValue: `C:\A different installation`})
	mustWrite(t, root, base+`\shell\open\command`, map[string]string{"": "custom command"})
	if err := unregister(root, dir); err != nil {
		t.Fatal(err)
	}
	assertString(t, root, base+`\shell\open\command`, "", "custom command")
}

func TestModifiedImmutableIconStopsBeforeRegistryWrites(t *testing.T) {
	root, dir := isolatedRegistry(t), applicationFixture(t)
	group := filetypes.Groups()[0]
	if err := os.WriteFile(filepath.Join(dir, group.IconName()), []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := register(root, dir, dir); err == nil {
		t.Fatal("overwrote an immutable icon")
	}
	assertMissingKey(t, root, "Software")
}

func TestWindowsLoadsEveryFileIconAtNativeSizes(t *testing.T) {
	dir := applicationFixture(t)
	if err := writeIcons(dir); err != nil {
		t.Fatal(err)
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	load, destroy := user32.NewProc("LoadImageW"), user32.NewProc("DestroyIcon")
	for _, group := range filetypes.Groups() {
		path, _ := windows.UTF16PtrFromString(filepath.Join(dir, group.IconName()))
		for _, size := range []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256} {
			icon, _, err := load.Call(0, uintptr(unsafe.Pointer(path)), 1, uintptr(size), uintptr(size), 0x10)
			if icon == 0 {
				t.Fatalf("%s cannot load at %dpx: %v", group.ID, size, err)
			}
			destroy.Call(icon)
		}
	}
}

func TestStoredCatalogCannotEscapeAssociationKeys(t *testing.T) {
	root := isolatedRegistry(t)
	mustWrite(t, root, statePath, map[string]string{"Directory": `C:\App`, "Source": `C:\App`})
	key, _ := registry.OpenKey(root, statePath, registry.SET_VALUE)
	if err := key.SetStringsValue("Extensions", []string{`.txt\..\Other`}); err != nil {
		t.Fatal(err)
	}
	key.Close()
	if _, err := readState(root); err == nil {
		t.Fatal("accepted an unsafe stored extension")
	}
}
