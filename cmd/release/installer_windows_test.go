package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestUniversalInstallerInstallsNativePayload(t *testing.T) {
	if os.Getenv("TEXTVIEWER_TEST_INSTALLER") != "1" {
		t.Skip("set TEXTVIEWER_TEST_INSTALLER=1 after building both payloads")
	}
	previous, _ := os.Getwd()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	config, err := readConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	identity := strings.ToLower(rand.Text())
	config.Name += "-InstallerTest-" + identity
	config.Identifier += ".test." + identity
	build := filepath.Join("dist", "native-"+config.Version)
	if override := os.Getenv("TEXTVIEWER_TEST_BUILD"); override != "" {
		build = override
	}
	setup := filepath.Join(work, "test-setup.exe")
	if err = buildInstaller(config, build, setup, true); err != nil {
		t.Fatal(err)
	}
	install := filepath.Join(work, "native payload with spaces")
	runSilentInstaller(t, setup, "/S /D="+install, 90*time.Second)
	uninstaller := filepath.Join(install, "Uninstall.exe")
	defer func() {
		if _, err := os.Stat(uninstaller); err == nil {
			runSilentInstaller(t, uninstaller, "/S _?="+install, 60*time.Second)
		}
	}()
	native := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
	var processMachine, nativeMachine uint16
	if windows.IsWow64Process2(windows.CurrentProcess(), &processMachine, &nativeMachine) == nil && nativeMachine == pe.IMAGE_FILE_MACHINE_ARM64 {
		native = pe.IMAGE_FILE_MACHINE_ARM64
	}
	for _, name := range bundleFiles {
		binary, err := pe.Open(filepath.Join(install, name))
		if err != nil {
			t.Fatal(err)
		}
		machine := binary.Machine
		binary.Close()
		if machine != native {
			t.Fatalf("%s machine %x, expected native %x", name, machine, native)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(install, "TextViewer.exe"), "--version")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.Output()
	if err != nil || strings.TrimSpace(string(output)) != config.Version {
		t.Fatalf("installed version: %q %v", output, err)
	}
	for _, name := range bundleFiles {
		if _, err := os.Stat(filepath.Join(install, name)); err != nil {
			t.Fatalf("installed bundle missing %s: %v", name, err)
		}
	}
	installedFiles, err := os.ReadDir(install)
	if err != nil || len(installedFiles) != 2 {
		t.Fatalf("installation should contain only TextViewer.exe and Uninstall.exe: %v %v", installedFiles, err)
	}
	if err := validatePayload(install, config.Version, map[uint16]string{pe.IMAGE_FILE_MACHINE_AMD64: "amd64", pe.IMAGE_FILE_MACHINE_ARM64: "arm64"}[native]); err != nil {
		t.Fatal(err)
	}
	executable, err := os.ReadFile(filepath.Join(install, "TextViewer.exe"))
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range filetypes.Groups() {
		icon, err := os.ReadFile(filepath.Join("internal", "fileassoc", "icons", group.IconName()))
		if err != nil || !bytes.Contains(executable, icon) {
			t.Fatalf("installed executable is missing the embedded %s file icon: %v", group.ID, err)
		}
	}
	userFile := filepath.Join(install, "user-created.txt")
	if err = os.WriteFile(userFile, []byte("preserve this"), 0600); err != nil {
		t.Fatal(err)
	}
	runSilentInstaller(t, uninstaller, "/S _?="+install, 60*time.Second)
	if _, err = os.Stat(filepath.Join(install, "TextViewer.exe")); !os.IsNotExist(err) {
		t.Fatal("uninstaller left the executable")
	}
	if data, err := os.ReadFile(userFile); err != nil || string(data) != "preserve this" {
		t.Fatal("uninstaller deleted a user-created file")
	}
	key := `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + config.Identifier
	if handle, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE); err == nil {
		handle.Close()
		t.Fatal("uninstaller left the Apps registry entry")
	}
	shortcut := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", config.Name+".lnk")
	if _, err := os.Stat(shortcut); !os.IsNotExist(err) {
		t.Fatal("uninstaller left the Start menu shortcut")
	}
}

func runSilentInstaller(t *testing.T, path, arguments string, timeout time.Duration) {
	t.Helper()
	if strings.ContainsAny(path, "\"\r\n") || strings.ContainsAny(arguments, "\"\r\n") {
		t.Fatal("unsafe installer test command line")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, path)
	// NSIS requires /D= and _?= to be the unquoted final argument.
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: syscall.EscapeArg(path) + " " + arguments}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installer %s: %v\n%s", filepath.Base(path), err, output)
	}
}
