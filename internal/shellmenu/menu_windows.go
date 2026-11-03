package shellmenu

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Oranjekop/TextViewer/internal/shellbundle"
	"github.com/Oranjekop/TextViewer/locales"
)

var ErrTrustRequired = locales.NewError(locales.SignatureTrustRequired)

var preparedBundle = sync.OnceValues(func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	root, err := integrationRoot()
	if err != nil {
		return "", err
	}
	return shellbundle.Prepare(exe, root)
})

func integrationRoot() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "TextViewer", "Integration", "bundles"), nil
}

func run(action string, extra ...string) (string, error) {
	dir, err := PrepareBundle()
	if err != nil {
		return "", err
	}
	script := filepath.Join(dir, "ContextMenu.ps1")
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", action, "-AppDirectory", dir}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, powershell, args...)
	cmd.Env = powerShellEnvironment()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "TEXTVIEWER_CERTIFICATE_TRUST_REQUIRED") {
			return "", ErrTrustRequired
		}
		return "", locales.NewError(func() string { return locales.WindowsIntegrationFailed(strings.TrimSpace(string(out))) })
	}
	return strings.TrimSpace(string(out)), nil
}

func powerShellEnvironment() []string {
	// Windows PowerShell cannot load PowerShell 7 modules inherited from a terminal.
	var environment []string
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if !strings.EqualFold(name, "PSModulePath") {
			environment = append(environment, variable)
		}
	}
	return environment
}

// TrustAndInstall is called only after the user confirms why an administrator
// prompt is needed. It trusts this end-entity signer, never a root CA.
func TrustAndInstall() error {
	dir, err := PrepareBundle()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "TextViewer.ContextMenu.cer"))
	if err != nil {
		return err
	}
	certificate, err := x509.ParseCertificate(data)
	if err != nil {
		return err
	}
	if certificate.IsCA {
		return locales.NewError(locales.CannotTrustRoot)
	}
	digest := sha1.Sum(certificate.Raw)
	if _, err = run("Trust", "-ExpectedThumbprint", strings.ToUpper(hex.EncodeToString(digest[:]))); err != nil {
		return err
	}
	return Install()
}

func Install() error   { _, err := run("Install"); return err }
func Uninstall() error { _, err := run("Remove"); return err }
func Cleanup() error   { _, err := run("Cleanup"); return err }
func Installed() (bool, error) {
	out, err := run("Status")
	return strings.Contains(out, "INSTALLED"), err
}

// PrepareBundle extracts the self-contained executable only when an integration
// is explicitly configured, without registering or trusting MSIX.
func PrepareBundle() (string, error) { return preparedBundle() }

func BundleDirectory() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	root, err := integrationRoot()
	if err != nil {
		return "", err
	}
	return shellbundle.Directory(exe, root)
}
