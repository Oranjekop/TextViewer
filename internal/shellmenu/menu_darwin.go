package shellmenu

import (
	"github.com/Oranjekop/TextViewer/internal/macsystem"
	"github.com/Oranjekop/TextViewer/locales"
)

var ErrTrustRequired = locales.NewError(locales.SignatureTrustRequired)

func TrustAndInstall() error { return Install() }
func Cleanup() error         { return nil }

func Install() error {
	m, err := macsystem.Current()
	if err != nil {
		return err
	}
	return m.ContextMenu(true, locales.FinderOpen())
}

func Uninstall() error {
	m, err := macsystem.Current()
	if err != nil {
		return err
	}
	return m.ContextMenu(false, "")
}

func Installed() (bool, error) {
	m, err := macsystem.Current()
	if err != nil {
		return false, err
	}
	return m.Installed()
}
