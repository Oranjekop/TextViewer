package document

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkLoad100MiB(b *testing.B) {
	for _, ending := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		b.Run(ending.name, func(b *testing.B) {
			line := strings.Repeat("TextViewer native editor / 中文文本 / ", 3) + ending.value
			chunk := strings.Repeat(line, 8192)
			path := filepath.Join(b.TempDir(), "large.txt")
			f, err := os.Create(path)
			if err != nil {
				b.Fatal(err)
			}
			size := 0
			for size < 100<<20 {
				n, err := f.WriteString(chunk)
				if err != nil {
					b.Fatal(err)
				}
				size += n
			}
			if err := f.Close(); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				doc, err := Load(context.Background(), path)
				if err != nil {
					b.Fatal(err)
				}
				if doc.Buffer.LineCount() < 1000 {
					b.Fatal("fixture was not loaded")
				}
			}
		})
	}
}
