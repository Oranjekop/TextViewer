package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestEditorURL(t *testing.T) {
	for _, tc := range []struct{ text, click, want string }{
		{"See https://example.com/path?q=1#part today", "example", "https://example.com/path?q=1#part"},
		{"[link](https://example.com/a_(b)).", "example", "https://example.com/a_(b)"},
		{"中文 https://example.com/中文。", "example", "https://example.com/中文"},
		{"www.example.com/test", "example", "https://www.example.com/test"},
		{"http://localhost:8080/test", "localhost", "http://localhost:8080/test"},
		{"javascript:alert(1)", "alert", ""},
		{"https://", "https", ""},
		{"https://user:password@example.com", "example", ""},
		{"plain text", "text", ""},
		{strings.Repeat("x", 10000) + " https://example.com", "example", "https://example.com"},
	} {
		var buffer editor.Buffer
		buffer.Set(tc.text)
		if got := editorURL(buffer.Snapshot(), strings.Index(tc.text, tc.click)); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.text[:min(len(tc.text), 100)], got, tc.want)
		}
	}
}
