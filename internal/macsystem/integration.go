// Package macsystem manages only TextViewer's per-user macOS integrations.
package macsystem

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	pbs        = "/System/Library/CoreServices/pbs"
	owner      = "local.oranjekop.textviewer.quick-action"
)

type Manager struct {
	Bundle, Support, Services string
	Run                       func(string, ...string) error
}

func Current() (Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return Manager{}, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return Manager{}, err
	}
	bundle, err := BundlePath(executable)
	if err != nil {
		return Manager{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Manager{}, err
	}
	return Manager{
		Bundle: bundle, Support: filepath.Join(home, "Library", "Application Support", "TextViewer"),
		Services: filepath.Join(home, "Library", "Services"),
		Run: func(name string, args ...string) error {
			output, err := exec.Command(name, args...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(string(output)))
			}
			return nil
		},
	}, nil
}

func BundlePath(executable string) (string, error) {
	for path := filepath.Dir(executable); path != filepath.Dir(path); path = filepath.Dir(path) {
		if strings.EqualFold(filepath.Ext(path), ".app") {
			if _, err := os.Stat(filepath.Join(path, "Contents", "Info.plist")); err == nil {
				return path, nil
			}
		}
	}
	return "", errors.New("run TextViewer from its installed .app bundle")
}

func (m Manager) Registered() (bool, error) {
	data, err := os.ReadFile(filepath.Join(m.Support, "launchservices"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return string(data) == m.Bundle, err
}

func (m Manager) Associations(enabled bool) error {
	flag := "-u"
	if enabled {
		flag = "-f"
	}
	if err := m.Run(lsregister, flag, m.Bundle); err != nil {
		return err
	}
	path := filepath.Join(m.Support, "launchservices")
	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(m.Support, 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(m.Bundle), 0600)
}

func (m Manager) workflow() string {
	return filepath.Join(m.Services, "Open in TextViewer.workflow")
}

func (m Manager) Installed() (bool, error) {
	data, err := os.ReadFile(filepath.Join(m.workflow(), "Contents", "TextViewer-owner"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if string(data) != owner {
		return false, errors.New("Finder workflow is not owned by TextViewer")
	}
	for _, file := range []string{"Info.plist", "document.wflow"} {
		if _, err := os.Stat(filepath.Join(m.workflow(), "Contents", file)); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (m Manager) ContextMenu(enabled bool, title string) error {
	path := m.workflow()
	if _, err := os.Lstat(path); err == nil {
		// Never follow a workflow symlink or overwrite another application's files.
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Finder workflow path is not a regular directory")
		}
		contents, err := os.Lstat(filepath.Join(path, "Contents"))
		if err != nil || !contents.IsDir() || contents.Mode()&os.ModeSymlink != 0 {
			return errors.New("Finder workflow contents are not a regular directory")
		}
		data, err := os.ReadFile(filepath.Join(path, "Contents", "TextViewer-owner"))
		if err != nil || string(data) != owner {
			return errors.New("an existing Finder workflow is not owned by TextViewer")
		}
		for _, name := range []string{"Info.plist", "document.wflow", "TextViewer-owner"} {
			info, err := os.Lstat(filepath.Join(path, "Contents", name))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err == nil && !info.Mode().IsRegular() {
				return errors.New("Finder workflow contains a nonregular managed file")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !enabled {
		for _, name := range []string{"Info.plist", "document.wflow", "TextViewer-owner"} {
			err := os.Remove(filepath.Join(path, "Contents", name))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		// Remove empty directories only; preserve any user-added content.
		_ = os.Remove(filepath.Join(path, "Contents"))
		_ = os.Remove(path)
	} else {
		contents := filepath.Join(path, "Contents")
		if err := os.MkdirAll(contents, 0700); err != nil {
			return err
		}
		files := map[string]string{
			"TextViewer-owner": owner,
			"Info.plist": plist(`<key>CFBundleIdentifier</key><string>` + owner + `</string>
<key>NSServices</key><array><dict>
<key>NSMenuItem</key><dict><key>default</key><string>` + escape(title) + `</string></dict>
<key>NSMessage</key><string>runWorkflowAsService</string>
<key>NSRequiredContext</key><dict><key>NSApplicationIdentifier</key><string>com.apple.finder</string></dict>
<key>NSSendFileTypes</key><array><string>public.item</string></array>
</dict></array>`),
			"document.wflow": workflowDocument(m.Bundle),
		}
		for _, name := range []string{"TextViewer-owner", "Info.plist", "document.wflow"} {
			if err := os.WriteFile(filepath.Join(contents, name), []byte(files[name]), 0600); err != nil {
				return err
			}
		}
	}
	return m.Run(pbs, "-update")
}

func escape(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func plist(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>` + body + `</dict></plist>`
}

func workflowDocument(bundle string) string {
	script := `exec /usr/bin/open -a '` + strings.ReplaceAll(bundle, "'", `'\''`) + `' -- "$@"`
	return plist(`<key>AMApplicationBuild</key><string>523</string>
<key>AMApplicationVersion</key><string>2.10</string>
<key>AMDocumentVersion</key><string>2</string>
<key>actions</key><array><dict><key>action</key><dict>
<key>AMActionVersion</key><string>2.0.3</string>
<key>AMAccepts</key><dict><key>Container</key><string>List</string><key>Types</key><array><string>com.apple.cocoa.path</string></array></dict>
<key>AMProvides</key><dict><key>Container</key><string>List</string><key>Types</key><array><string>com.apple.cocoa.string</string></array></dict>
<key>ActionBundlePath</key><string>/System/Library/Automator/Run Shell Script.action</string>
<key>ActionName</key><string>Run Shell Script</string>
<key>ActionParameters</key><dict>
<key>COMMAND_STRING</key><string>` + escape(script) + `</string>
<key>CheckedForUserDefaultShell</key><true/><key>inputMethod</key><integer>1</integer>
<key>shell</key><string>/bin/sh</string></dict>
<key>BundleIdentifier</key><string>com.apple.RunShellScript</string>
<key>UUID</key><string>A57C7630-788D-44A4-97CF-CF9943596062</string>
</dict></dict></array><key>connectors</key><dict/>
<key>workflowMetaData</key><dict>
<key>applicationBundleID</key><string>com.apple.finder</string>
<key>applicationBundleIDsByPath</key><dict/>
<key>inputTypeIdentifier</key><string>com.apple.Automator.fileSystemObject</string>
<key>outputTypeIdentifier</key><string>com.apple.Automator.nothing</string>
<key>presentationMode</key><integer>11</integer>
<key>processesInput</key><false/><key>serviceInputTypeIdentifier</key><string>com.apple.Automator.fileSystemObject</string>
<key>serviceOutputTypeIdentifier</key><string>com.apple.Automator.nothing</string>
<key>serviceProcessesInput</key><false/><key>workflowTypeIdentifier</key><string>com.apple.Automator.servicesMenu</string>
</dict>`)
}
