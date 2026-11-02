//go:build !windows

package document

import (
	"os"
)

func SamePath(a, b string) bool {
	a, _ = ResolvePath(a)
	b, _ = ResolvePath(b)
	return a == b
}

func replaceFile(source, target string) error { return os.Rename(source, target) }
