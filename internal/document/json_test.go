package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

func TestFormatJSONPreservesLexemes(t *testing.T) {
	for _, input := range []string{
		`{"z":9007199254740993123456789,"a":1.00e+20,"z":-0,"text":"中文 \u4e2d \/ \t"}`,
		` [ {}, [], [1,2], {"a":"\\\"{},[]"} ] ` + "\t\n",
		" \n [true,false,null,0.0,-12e-3,\"😀\"]",
		`"plain \u0061"`,
		`1.000e+20`,
		`{}`,
		"[\n  1,\n  2\n]\n",
	} {
		t.Run(input, func(t *testing.T) {
			snapshot := editor.NewBuffer(input).Snapshot()
			var expected bytes.Buffer
			if err := json.Indent(&expected, []byte(input), "", "  "); err != nil {
				t.Fatal(err)
			}
			output, changed, err := FormatJSON(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if !changed {
				output = input
			}
			if output != expected.String() || changed != (output != input) {
				t.Fatalf("format mismatch: %q / %q changed=%v", output, expected.String(), changed)
			}
			bound, err := jsonOutputBound(context.Background(), []byte(input))
			if err != nil || len(output) > bound {
				t.Fatalf("incorrect output bound: length=%d bound=%d err=%v", len(output), bound, err)
			}
		})
	}
}

func TestFormatJSONRejectsInvalidOrCanceledInput(t *testing.T) {
	for _, input := range []string{
		"", `{"a":}`, `{"a":1,}`, `{"a":NaN}`, `{"a":1}{"b":2}`, "/* jsonc */ {}", "[1,\n2",
	} {
		buffer := editor.NewBuffer(input)
		snapshot := buffer.Snapshot()
		if output, changed, err := FormatJSON(context.Background(), snapshot); err == nil || changed || output != "" {
			t.Fatalf("invalid input accepted: %q => %q changed=%v err=%v", input, output, changed, err)
		}
		if buffer.Snapshot() != snapshot || buffer.CanUndo() {
			t.Fatal("failed format modified the document")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := FormatJSON(ctx, editor.NewBuffer(`{"a":1}`).Snapshot()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not honored: %v", err)
	}
}

func TestJSONIndentationExpansionIsBounded(t *testing.T) {
	input := strings.Repeat("[", 1000) + strings.Repeat("0,", 160000) + "0" + strings.Repeat("]", 1000)
	if !json.Valid([]byte(input)) {
		t.Fatal("fixture should be valid JSON")
	}
	buffer := editor.NewBuffer(input)
	snapshot := buffer.Snapshot()
	if _, changed, err := FormatJSON(context.Background(), snapshot); err == nil || changed ||
		!strings.Contains(err.Error(), "256 MiB") {
		t.Fatalf("excessive output was not bounded: changed=%v err=%v", changed, err)
	}
	if buffer.Snapshot() != snapshot {
		t.Fatal("rejected expansion changed the input")
	}
}

func BenchmarkFormatJSON100MiB(b *testing.B) {
	line := `{"text":"TextViewer 中文","number":9007199254740993,"flag":true},` + "\n"
	input := "[" + strings.Repeat(line, (100<<20)/len(line)) + "{}]\n"
	snapshot := editor.NewBuffer(input).Snapshot()
	b.SetBytes(int64(snapshot.Len()))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, changed, err := FormatJSON(context.Background(), snapshot)
		if err != nil || !changed {
			b.Fatalf("format: %v changed=%v", err, changed)
		}
	}
}

func FuzzFormatJSONMatchesStandardIndent(f *testing.F) {
	for _, seed := range []string{
		`{"z":9007199254740993,"a":[{},[],true,null],"z":"\\\"\u4e2d"}`,
		"[ 1, { \"x\" : -0.00e+12 }, [ ] ]\r\n",
		`"中文 😀"`, "null \t\n", `0`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1<<20 || !utf8.ValidString(input) || !json.Valid([]byte(input)) {
			t.Skip()
		}
		snapshot := editor.NewBuffer(input).Snapshot()
		original := snapshot.String()
		output, changed, err := FormatJSON(context.Background(), snapshot)
		if err != nil {
			if strings.Contains(err.Error(), "256 MiB") {
				t.Skip()
			}
			t.Fatal(err)
		}
		var expected bytes.Buffer
		if err := json.Indent(&expected, []byte(original), "", "  "); err != nil {
			t.Fatal(err)
		}
		if !changed {
			output = original
		}
		if output != expected.String() {
			t.Fatalf("mismatched formatting for %q: %q != %q", original, output, expected.String())
		}
	})
}
