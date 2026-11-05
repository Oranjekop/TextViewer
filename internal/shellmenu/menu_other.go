//go:build !windows && !darwin

package shellmenu

import "github.com/Oranjekop/TextViewer/locales"

var ErrTrustRequired = locales.NewError(locales.SignatureTrustRequired)

func TrustAndInstall() error { return Install() }

func Install() error           { return locales.NewError(locales.RequiresWindows) }
func Uninstall() error         { return locales.NewError(locales.RequiresWindows) }
func Cleanup() error           { return nil }
func Installed() (bool, error) { return false, nil }
