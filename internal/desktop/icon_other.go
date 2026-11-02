//go:build !windows

package desktop

import (
	"github.com/Oranjekop/TextViewer/assets"
	"github.com/egoist/mygo"
)

func configureWindowIcon(window *mygo.Window) error {
	return window.SetIcon(assets.AppIcon)
}
