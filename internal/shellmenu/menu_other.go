//go:build !windows

package shellmenu

import "errors"

var ErrTrustRequired = errors.New("需要先信任 TextViewer 的本机签名")

func TrustAndInstall() error { return Install() }

func Install() error           { return errors.New("此功能需要 Windows 11") }
func Uninstall() error         { return errors.New("此功能需要 Windows 11") }
func Installed() (bool, error) { return false, nil }
