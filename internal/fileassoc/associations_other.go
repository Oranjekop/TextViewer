//go:build !windows && !darwin

package fileassoc

import "github.com/Oranjekop/TextViewer/locales"

func Register() error                    { return locales.NewError(locales.AssociationsRequireWindows) }
func RegisterSelection(_ []string) error { return Register() }
func Unregister() error                  { return locales.NewError(locales.AssociationsRequireWindows) }
func OpenDefaults() error                { return locales.NewError(locales.DefaultsRequireWindows) }
func Registered() (bool, error)          { return false, nil }
func RemoveRegistration() error          { return Unregister() }
