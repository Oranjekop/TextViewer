package desktop

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

func (a *App) openEditorURL(offset int) {
	if target := editorURL(a.doc.Buffer.Snapshot(), offset); target != "" {
		if err := mygo.Shell.OpenExternal(target); err != nil {
			a.status = err.Error
		}
	}
}

func editorURL(text editor.Snapshot, offset int) string {
	if offset < 0 || offset >= text.Len() {
		return ""
	}
	// Bound work even for a huge single-line log; never copy the document.
	const limit = 8192
	start, end := max(0, offset-limit), min(text.Len(), offset+limit)
	s := text.Slice(start, end)
	index := offset - start
	delimiter := func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("\"'<>`", r)
	}
	left := strings.LastIndexFunc(s[:index], delimiter) + 1
	right := len(s)
	if n := strings.IndexFunc(s[index:], delimiter); n >= 0 {
		right = index + n
	}
	if left == 0 && start > 0 || right == len(s) && end < text.Len() {
		return ""
	}
	candidate := s[left:right]
	lower := strings.ToLower(candidate)
	begin := -1
	for _, prefix := range []string{"https://", "http://", "www."} {
		if n := strings.Index(lower, prefix); n >= 0 && (begin < 0 || n < begin) {
			begin = n
		}
	}
	if begin < 0 || index < left+begin {
		return ""
	}
	candidate = strings.TrimRight(candidate[begin:], ".,;:!?，。；！")
	for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
		for strings.HasSuffix(candidate, pair[1]) && strings.Count(candidate, pair[1]) > strings.Count(candidate, pair[0]) {
			candidate = strings.TrimSuffix(candidate, pair[1])
		}
	}
	if index >= left+begin+len(candidate) {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(candidate), "www.") {
		candidate = "https://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil || u.Hostname() == "" || u.User != nil || (strings.ToLower(u.Scheme) != "http" && strings.ToLower(u.Scheme) != "https") {
		return ""
	}
	return candidate
}
