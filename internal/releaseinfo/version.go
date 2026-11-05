package releaseinfo

import (
	"fmt"
	"strconv"
	"strings"
)

const Repository = "Oranjekop/TextViewer"

func ParseVersion(value string) ([3]uint16, error) {
	var version [3]uint16
	parts := strings.Split(value, ".")
	if len(parts) != len(version) {
		return version, fmt.Errorf("invalid stable version %q", value)
	}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return version, fmt.Errorf("invalid version component %q", part)
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return version, fmt.Errorf("invalid version component %q", part)
			}
		}
		n, err := strconv.ParseUint(part, 10, 16)
		if err != nil {
			return version, err
		}
		version[i] = uint16(n)
	}
	return version, nil
}

func TagVersion(tag string) (string, error) {
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("release tag must be vMAJOR.MINOR.PATCH")
	}
	version := strings.TrimPrefix(tag, "v")
	_, err := ParseVersion(version)
	return version, err
}

func Newer(next, current string) (bool, error) {
	n, err := ParseVersion(next)
	if err != nil {
		return false, err
	}
	c, err := ParseVersion(current)
	if err != nil {
		return false, err
	}
	for i := range n {
		if n[i] != c[i] {
			return n[i] > c[i], nil
		}
	}
	return false, nil
}

func InstallerName(version string) string {
	return "TextViewer-Setup-" + version + "-windows.exe"
}

func InstallerNameFor(version, platform string) string {
	if platform == "darwin" {
		return "TextViewer-Setup-" + version + "-macos-universal.pkg"
	}
	if platform == "windows" {
		return InstallerName(version)
	}
	return ""
}

func ArchiveName(version, arch string) string {
	return "TextViewer-" + version + "-windows-" + arch + ".zip"
}
