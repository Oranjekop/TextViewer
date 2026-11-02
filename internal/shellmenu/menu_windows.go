package shellmenu

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var ErrTrustRequired = errors.New("需要先信任 TextViewer 的本机签名")

func run(action string, extra ...string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	script := filepath.Join(dir, "ContextMenu.ps1")
	if _, err = os.Stat(script); err != nil {
		return "", errors.New("请使用完整构建目录中的 TextViewer.exe，右键扩展需要同目录的安装文件")
	}
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", action, "-AppDirectory", dir}
	args = append(args, extra...)
	cmd := exec.CommandContext(ctx, powershell, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "TEXTVIEWER_CERTIFICATE_TRUST_REQUIRED") {
			return "", ErrTrustRequired
		}
		return "", fmt.Errorf("右键菜单设置失败：%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// TrustAndInstall is called only after the user confirms why an administrator
// prompt is needed. It trusts this end-entity signer, never a root CA.
func TrustAndInstall() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "TextViewer.ContextMenu.cer"))
	if err != nil {
		return err
	}
	certificate, err := x509.ParseCertificate(data)
	if err != nil {
		return err
	}
	if certificate.IsCA {
		return errors.New("不允许信任根证书")
	}
	digest := sha1.Sum(certificate.Raw)
	if _, err = run("Trust", "-ExpectedThumbprint", strings.ToUpper(hex.EncodeToString(digest[:]))); err != nil {
		return err
	}
	return Install()
}

func Install() error   { _, err := run("Install"); return err }
func Uninstall() error { _, err := run("Remove"); return err }
func Installed() (bool, error) {
	out, err := run("Status")
	return strings.Contains(out, "INSTALLED"), err
}
