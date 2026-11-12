package fileassoc

import (
	"os/exec"

	"github.com/Oranjekop/TextViewer/internal/macsystem"
	"github.com/Oranjekop/TextViewer/locales"
)

func Register() error {
	m, err := macsystem.Current()
	if err != nil {
		return err
	}
	return m.Associations(true)
}

func RegisterSelection(extensions []string) error {
	if extensions != nil {
		return locales.NewError(locales.AssociationsRequireWindows)
	}
	return Register()
}

func Unregister() error {
	m, err := macsystem.Current()
	if err != nil {
		return err
	}
	return m.Associations(false)
}

func Registered() (bool, error) {
	m, err := macsystem.Current()
	if err != nil {
		return false, err
	}
	return m.Registered()
}

func RemoveRegistration() error { return Unregister() }

func OpenDefaults() error {
	return exec.Command("/usr/bin/open", "-b", "com.apple.finder").Run()
}
