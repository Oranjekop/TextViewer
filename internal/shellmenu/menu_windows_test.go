package shellmenu

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPortableBundlePreparationDoesNotRequireMSIXTrust(t *testing.T) {
	work := t.TempDir()
	source := filepath.Join(work, "中文 portable folder")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "shell", "ContextMenu.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"TextViewer.exe", "TextViewerShell.dll", "TextViewer.ContextMenu.msix", "TextViewer.ContextMenu.cer", "ContextMenu.ps1"}
	for _, name := range files {
		data := []byte("fixture only; not an executable, certificate or package")
		if name == "ContextMenu.ps1" {
			data = script
		} else if name == "TextViewer.ContextMenu.msix" {
			var buffer bytes.Buffer
			archive := zip.NewWriter(&buffer)
			for _, asset := range []string{"Logo.png", "Logo44.png", "FileTypeText.png"} {
				entry, err := archive.Create("Assets/" + asset)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write([]byte("packaged " + asset)); err != nil {
					t.Fatal(err)
				}
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			data = buffer.Bytes()
		}
		if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cache := filepath.Join(work, "isolated local app data")
	run := func(action string) (string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-File", filepath.Join(source, "ContextMenu.ps1"), "-Action", action, "-AppDirectory", source)
		cmd.Env = append(powerShellEnvironment(), "LOCALAPPDATA="+cache)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, err := cmd.CombinedOutput()
		return strings.TrimPrefix(strings.TrimSpace(string(out)), "PREPARED "), err
	}
	dir, err := run("Locate")
	if err != nil {
		t.Fatal(dir, err)
	}
	if !strings.HasPrefix(dir, cache+string(filepath.Separator)) {
		t.Fatalf("bundle escaped the isolated test directory: %s", dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("locating a bundle created files")
	}
	start := time.Now()
	prepared, err := run("Prepare")
	if err != nil || prepared != dir {
		t.Fatalf("bundle required trust or returned another path: %s %v", prepared, err)
	}
	t.Logf("Prepared immutable portable bundle in %s (small fixture; includes PowerShell startup)", time.Since(start))
	for _, name := range files {
		from, _ := os.ReadFile(filepath.Join(source, name))
		to, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(from) != string(to) {
			t.Fatalf("missing or changed immutable %s: %v", name, err)
		}
	}
	for _, asset := range []string{"Logo.png", "Logo44.png", "FileTypeText.png"} {
		data, err := os.ReadFile(filepath.Join(dir, "Assets", asset))
		if err != nil || string(data) != "packaged "+asset {
			t.Fatalf("missing external identity visual asset %s: %s %v", asset, data, err)
		}
	}
	repeated, err := run("Prepare")
	if err != nil || repeated != dir {
		t.Fatal("repeated preparation failed:", repeated, err)
	}
	// A new build gets another directory and preserves the previous bundle.
	if err := os.WriteFile(filepath.Join(source, "TextViewer.exe"), []byte("fixture version two"), 0600); err != nil {
		t.Fatal(err)
	}
	newDir, err := run("Prepare")
	if err != nil || newDir == dir {
		t.Fatal("new executable reused the old bundle:", newDir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "TextViewer.exe")); err != nil {
		t.Fatal("preparing a new bundle removed the old one:", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "TextViewerShell.dll"), []byte("modified immutable DLL"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("Prepare"); err == nil {
		t.Fatal("overwrote immutable files:", out)
	}
}
