package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareDerivesVersionFromTagWithoutDroppingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mygo.json")
	if err := os.WriteFile(path, []byte(`{"name":"TextViewer","version":"0.0.1","windows":{"installerInclude":"shell/installer.nsh"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareVersion(path, "0.0.2"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if string(config["version"]) != `"0.0.2"` || !strings.Contains(string(config["windows"]), "installer.nsh") {
		t.Fatalf("configuration lost fields: %s", data)
	}
	if err := prepareVersion(path, "0.0.3-rc.1"); err == nil {
		t.Fatal("accepted a version not supported by the Windows installer")
	}
}

func fakePayload(t *testing.T, root string) {
	t.Helper()
	for _, arch := range []string{"amd64", "arm64"} {
		dir := filepath.Join(root, "windows-"+arch)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range bundleFiles {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture-"+arch+"-"+name), 0600); err != nil {
				t.Fatal(err)
			}
		}
		os.WriteFile(filepath.Join(dir, "textviewer-setup-0.0.1-"+arch+".exe"), []byte("not a payload"), 0600)
	}
}

func TestArchivesContainCompleteBundleButNoNestedInstaller(t *testing.T) {
	root := t.TempDir()
	fakePayload(t, root)
	archive := filepath.Join(root, "portable.zip")
	if err := writeArchive(filepath.Join(root, "windows-amd64"), archive); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != len(bundleFiles) {
		t.Fatal("unexpected portable archive contents")
	}
	for _, entry := range reader.File {
		if strings.Contains(entry.Name, "/") || strings.Contains(entry.Name, "setup") {
			t.Fatal("archive has nested paths or installers")
		}
	}
}

func TestInstallerSelectsNativeArchitectureAndNeverRecursivelyDeletes(t *testing.T) {
	root := t.TempDir()
	fakePayload(t, root)
	script, err := installerScript(configuration{Name: "TextViewer", Identifier: "test.textviewer", Version: "0.0.1"}, root, filepath.Join(root, "setup.exe"), filepath.Join(root, "icon.ico"), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"${IsNativeARM64}", `"0.0.1.0"`, "TEXTVIEWER_ISOLATED_TEST",
		"RequestExecutionLevel user", `RMDir "$INSTDIR"`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("installer missing %s", required)
		}
	}
	if strings.Contains(script, "RMDir /r") || strings.Contains(script, "setup-0.0.1-") || strings.Contains(script, "Stop-Process") {
		t.Fatal("installer recursively removes user files, embeds another installer or kills the editor")
	}
	if strings.Count(script, "\n    File \"") != 2*len(bundleFiles) {
		t.Fatal("both native payloads were not embedded exactly once")
	}
}

func TestIdentityManifestMatchesReleaseVersionAndArchitecture(t *testing.T) {
	manifest := []byte(`<Package><Identity Name="Oranjekop.TextViewer.ContextMenu" Version="0.0.1.0" ProcessorArchitecture="arm64"/></Package>`)
	if err := validateIdentityManifest(manifest, "0.0.1", "arm64"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ version, arch string }{{"0.0.2", "arm64"}, {"0.0.1", "x64"}} {
		if err := validateIdentityManifest(manifest, tc.version, tc.arch); err == nil {
			t.Fatalf("accepted the wrong %s %s package", tc.version, tc.arch)
		}
	}
}
