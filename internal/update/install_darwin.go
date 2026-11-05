package update

import (
	"errors"
	"os/exec"
	"path/filepath"
)

func StartInstaller(installer *Installer) error {
	if err := installer.Verify(); err != nil {
		return err
	}
	if !filepath.IsAbs(installer.Path) || filepath.Ext(installer.Path) != ".pkg" {
		return errors.New("invalid macOS installer path")
	}
	cmd := exec.Command("/usr/bin/open", "-b", "com.apple.installer", installer.Path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
