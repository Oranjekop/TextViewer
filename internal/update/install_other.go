//go:build !windows

package update

import "errors"

func StartInstaller(installer *Installer) error {
	return errors.New("automatic installation is supported on Windows only")
}
