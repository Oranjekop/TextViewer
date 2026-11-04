package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(runDesktopTests(m))
}

func runDesktopTests(m *testing.M) int {
	root, err := os.MkdirTemp("", "TextViewer-desktop-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	temporary, err := filepath.Abs(os.TempDir())
	if err != nil || filepath.Dir(root) != temporary {
		fmt.Fprintln(os.Stderr, "desktop test configuration is outside the temporary directory")
		return 1
	}
	defer os.RemoveAll(root)
	// UI actions such as zoom save preferences, even in scrolling tests.
	for _, name := range []string{"APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR"} {
		if err := os.Setenv(name, root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}
