package macsystem

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testManager(t *testing.T) (Manager, *[][]string) {
	t.Helper()
	root := t.TempDir()
	var calls [][]string
	return Manager{
		Bundle:  filepath.Join(root, "TextViewer's & 中文.app"),
		Support: filepath.Join(root, "Support"), Services: filepath.Join(root, "Services"),
		Run: func(name string, args ...string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
	}, &calls
}

func TestIntegrationRoundTripAndOwnedRemoval(t *testing.T) {
	m, calls := testManager(t)
	if ok, err := m.Registered(); ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := m.Associations(true); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Registered(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := m.ContextMenu(true, "在 TextViewer 中打开"); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Installed(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	for _, name := range []string{"Info.plist", "document.wflow"} {
		data, err := os.ReadFile(filepath.Join(m.workflow(), "Contents", name))
		if err != nil {
			t.Fatal(err)
		}
		d := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			_, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if name == "document.wflow" && !strings.Contains(string(data), "&apos;\\&apos;&apos;") && !strings.Contains(string(data), "&#39;\\&#39;&#39;") {
			t.Fatal("bundle apostrophe not shell-escaped")
		}
	}
	extra := filepath.Join(m.workflow(), "Contents", "user-file")
	if err := os.WriteFile(extra, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.ContextMenu(false, ""); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(extra); err != nil || string(data) != "keep" {
		t.Fatal("removed user content")
	}
	if ok, err := m.Installed(); ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := m.Associations(false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*calls, [][]string{{lsregister, "-f", m.Bundle}, {pbs, "-update"}, {pbs, "-update"}, {lsregister, "-u", m.Bundle}}) {
		t.Fatalf("unexpected system operations: %v", *calls)
	}
}

func TestRefusesUnownedWorkflowAndFailedRegistration(t *testing.T) {
	m, _ := testManager(t)
	if err := os.MkdirAll(filepath.Join(m.workflow(), "Contents"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		if err := m.ContextMenu(enabled, "Open"); err == nil {
			t.Fatal("modified unowned workflow")
		}
	}
	m.Run = func(string, ...string) error { return errors.New("fixture failure") }
	if err := m.Associations(true); err == nil {
		t.Fatal("lost registration error")
	}
	if ok, _ := m.Registered(); ok {
		t.Fatal("persisted failed registration")
	}
}

func TestLocatesInstalledBundle(t *testing.T) {
	m, _ := testManager(t)
	if err := os.MkdirAll(filepath.Join(m.Bundle, "Contents"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Bundle, "Contents", "Info.plist"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := BundlePath(filepath.Join(m.Bundle, "Contents", "MacOS", "TextViewer")); err != nil || got != m.Bundle {
		t.Fatal(got, err)
	}
	if _, err := BundlePath(filepath.Join(t.TempDir(), "TextViewer")); err == nil {
		t.Fatal("accepted unbundled executable")
	}
}
