package update

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

func StartInstaller(installer *Installer) error {
	if err := installer.Verify(); err != nil {
		return err
	}
	path := installer.Path
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return errors.New("invalid installer path")
	}
	command := exec.Command(path)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
