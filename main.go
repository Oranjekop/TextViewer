package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Oranjekop/TextViewer/internal/desktop"
	"github.com/Oranjekop/TextViewer/internal/fileassoc"
	"github.com/Oranjekop/TextViewer/internal/instance"
	"github.com/Oranjekop/TextViewer/internal/shellmenu"
	"github.com/Oranjekop/TextViewer/internal/startuptrace"
	_ "github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo"
)

func main() {
	entered := time.Now()
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
	trace := startuptrace.New(version, entered)
	defer trace.Complete("exit")
	var app *desktop.App
	lock, forwarded, err := instance.Start(func(args []string, dir string) {
		mygo.App.WhenReady(func() { app.OpenArguments(args, dir) })
	})
	trace.Mark("instance-ready")
	if forwarded {
		trace.Complete("forwarded")
		return
	}
	if err != nil {
		// An unresponsive old editor must not swallow the user's launch.
		log.Printf("TextViewer: single instance activation: %v", err)
	}
	defer lock.Close()
	app = desktop.New()
	app.StartupTrace = trace
	trace.Mark("preferences-ready")
	mygo.App.WhenReady(func() {
		trace.Mark("backend-ready")
		if err := app.Ready(); err != nil {
			exitWith(err)
			return
		}
		app.OpenArguments(os.Args[1:], "")
	})
	mygo.App.OnOpenFile(fileOpenHandler(mygo.App.WhenReady, app.OpenArguments))
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
