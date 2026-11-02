package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/desktop"
	"github.com/Oranjekop/TextViewer/internal/fileassoc"
	"github.com/Oranjekop/TextViewer/internal/instance"
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
	app := desktop.New()
	lock, forwarded, err := instance.Start(func(args []string, dir string) {
		mygo.App.WhenReady(func() { app.OpenArguments(args, dir) })
	})
	if forwarded {
		return
	}
	if err != nil {
		// An unresponsive old editor must not swallow the user's launch.
		log.Printf("TextViewer: single instance activation: %v", err)
	}
	defer lock.Close()
	mygo.App.WhenReady(func() {
		if err := app.Ready(); err != nil {
			exitWith(err)
			return
		}
		app.OpenArguments(os.Args[1:], "")
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
