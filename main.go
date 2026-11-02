package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/desktop"
	"github.com/Oranjekop/TextViewer/internal/fileassoc"
	"github.com/Oranjekop/TextViewer/internal/shellmenu"
	"github.com/egoist/mygo"
)

func main() {
	version := applicationVersion()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version":
			fmt.Println(version)
			return
		case "--install-context-menu":
			exitWith(shellmenu.Install())
			return
		case "--uninstall-context-menu":
			exitWith(shellmenu.Uninstall())
			return
		case "--register-file-associations":
			exitWith(fileassoc.Register())
			return
		case "--unregister-file-associations":
			exitWith(fileassoc.Unregister())
			return
		case "--configure-windows-integration":
			exitWith(desktop.ConfigureInstalledIntegration())
			return
		case "--cleanup-old-versions":
			exitWith(shellmenu.Cleanup())
			return
		case "--context-menu-status":
			installed, err := shellmenu.Installed()
			if err != nil {
				exitWith(err)
			}
			fmt.Println(installed)
			return
		case "--performance-check":
			exitWith(desktop.PerformanceCheck(os.Args[2:]))
			return
		}
	}
	mygo.App.SetName("TextViewer")
	mygo.App.SetVersion(version)
	mygo.App.SetLocale("zh-CN")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app := desktop.New()
	mygo.App.WhenReady(func() {
		app.Ready()
		app.OpenArguments(os.Args[1:], "")
	})
	mygo.App.OnSecondInstance(func(args []string, dir string) {
		app.OpenArguments(args[1:], dir)
	})
	mygo.App.OnOpenFile(func(path string) {
		if !strings.HasPrefix(filepath.Base(path), "-") {
			app.OpenArguments([]string{path}, "")
		}
	})
	if err := mygo.App.Run(); err != nil {
		exitWith(err)
	}
}

func exitWith(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
