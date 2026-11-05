package textops

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestBase64Conversions(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		op                Operation
	}{
		{"encode", "中文😀\ntext", "5Lit5paH8J+YgAp0ZXh0", Base64Encode},
		{"decode", "5Lit5paH8J+YgAp0ZXh0", "中文😀\ntext", Base64Decode},
		{"padding", "YQ==", "a", Base64Decode},
		{"no padding", "YWI", "ab", Base64Decode},
		{"whitespace", " Y W\tJj\r\n ", "abc", Base64Decode},
		{"URL safe", "8J-YgA", "😀", Base64Decode},
		{"CRLF text", "YQ0KYg==", "a\nb", Base64Decode},
		{"empty", "", "", Base64Encode},
		{"empty decode", "", "", Base64Decode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := editor.NewBuffer(tc.input)
			original := buf.String()
			buf.SetLineEnding("\r\n")
			result, err := Prepare(context.Background(), buf.Snapshot(), nil, tc.op, 4)
			if err != nil {
				t.Fatal(err)
			}
			buf.Edit(result.Edits...)
			if buf.String() != tc.want {
				t.Fatalf("got %q want %q", buf.String(), tc.want)
			}
			if buf.LineEnding() != "\r\n" {
				t.Fatal("changed line ending")
			}
			if original != tc.want && (!buf.Undo() || buf.String() != original || buf.CanUndo()) {
				t.Fatal("conversion must be one undo step")
			}
		})
	}
	buf := editor.NewBuffer("YQ0KYg== Yw==")
	result, err := Prepare(context.Background(), buf.Snapshot(), []editor.Selection{
		{Head: 8}, {Anchor: 9, Head: 13},
	}, Base64Decode, 4)
	if err != nil {
		t.Fatal(err)
	}
	buf.Edit(result.Edits...)
	if buf.String() != "a\nb c" || result.Selections[1] != (editor.Selection{Anchor: 4, Head: 5}) {
		t.Fatalf("CRLF normalization broke subsequent selection: %+v", result)
	}
}

func TestBase64ExactSelectionsAndAtomicFailure(t *testing.T) {
	input := "keep YQ== middle Yg== end"
	buf := editor.NewBuffer(input)
	result, err := Prepare(context.Background(), buf.Snapshot(), []editor.Selection{
		{Anchor: 5, Head: 9}, {Anchor: 21, Head: 17},
	}, Base64Decode, 4)
	if err != nil {
		t.Fatal(err)
	}
	buf.Edit(result.Edits...)
	if buf.String() != "keep a middle b end" || len(result.Selections) != 2 ||
		result.Selections[0] != (editor.Selection{Anchor: 5, Head: 6}) ||
		result.Selections[1] != (editor.Selection{Anchor: 15, Head: 14}) {
		t.Fatalf("incorrect exact selections: %q %+v", buf.String(), result.Selections)
	}
	for _, invalid := range []string{"?", "/w==", "Zh==", "YQ=", "YQ==!"} {
		t.Run(invalid, func(t *testing.T) {
			buf := editor.NewBuffer("YQ== " + invalid)
			result, err := Prepare(context.Background(), buf.Snapshot(), []editor.Selection{
				{Anchor: 0, Head: 4}, {Anchor: 5, Head: buf.Len()},
			}, Base64Decode, 4)
			if err == nil || len(result.Edits) != 0 || buf.String() != "YQ== "+invalid {
				t.Fatalf("failure must not partially convert: %+v %v", result, err)
			}
		})
	}
}

func TestBase64LimitsAndCancellation(t *testing.T) {
	buf := editor.NewBuffer(strings.Repeat("a", maxBase64Bytes+1))
	if _, err := Prepare(context.Background(), buf.Snapshot(), nil, Base64Encode, 4); err == nil {
		t.Fatal("missing size limit")
	}
	// A small exact selection in a large file remains supported.
	if result, err := Prepare(context.Background(), buf.Snapshot(), []editor.Selection{{Head: 3}}, Base64Encode, 4); err != nil || len(result.Edits) != 1 {
		t.Fatalf("small selection rejected: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, buf.Snapshot(), nil, Base64Decode, 4); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func BenchmarkBase64EncodeMiB(b *testing.B) {
	snapshot := editor.NewBuffer(strings.Repeat("a", 1<<20)).Snapshot()
	b.SetBytes(int64(snapshot.Len()))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Prepare(context.Background(), snapshot, nil, Base64Encode, 4); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBase64DecodeMiB(b *testing.B) {
	snapshot := editor.NewBuffer(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 1<<20)))).Snapshot()
	b.SetBytes(int64(snapshot.Len()))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Prepare(context.Background(), snapshot, nil, Base64Decode, 4); err != nil {
			b.Fatal(err)
		}
	}
}
