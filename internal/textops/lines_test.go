package textops

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

func TestLineTransformsAndFinalNewlines(t *testing.T) {
	for _, tc := range []struct {
		op          Operation
		input, want string
	}{
		{SortAscending, "b\n\nA\na\n", "\nA\na\nb\n"},
		{SortDescending, "a\n中文\n😀\nb", "😀\n中文\nb\na"},
		{SortIgnoreCase, "b\na\nA\nB\n", "a\nA\nb\nB\n"},
		{ReverseLines, "a\n\nb\n", "b\n\na\n"},
		{UniqueLines, "b\na\nb\n\n\nA\n", "b\na\n\nA\n"},
		{UniqueLines, "\n\n", "\n"},
		{TrimLeading, " \t a  \n\tb\n", "a  \nb\n"},
		{TrimTrailing, "  a \t\nb\t \n", "  a\nb\n"},
		{TrimBoth, " \t a \t\n \tb \n", "a\nb\n"},
		{RemoveEmptyLines, "\na\n \t\n\nb\n\n", "a\n \t\nb\n"},
		{RemoveBlankLines, "\na\n \t\n\nb\n \t", "a\nb\n"},
		{RemoveBlankLines, "\n \t\n", ""},
		{TabsToSpaces, "\tA\tB\n中\t😀\t\n", "    A   B\n中   😀   \n"},
		{LeadingSpacesToTabs, "   \tx  y\n      z\n", "\tx  y\n\t  z\n"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			buffer := editor.NewBuffer(tc.input)
			buffer.SetLineEnding("\r\n")
			result, err := Prepare(context.Background(), buffer.Snapshot(), []editor.Selection{{}}, tc.op, 4)
			if err != nil {
				t.Fatal(err)
			}
			buffer.Edit(result.Edits...)
			if got := buffer.String(); got != tc.want {
				t.Fatalf("operation %d: got %q want %q", tc.op, got, tc.want)
			}
			if buffer.LineEnding() != "\r\n" {
				t.Fatal("output line ending changed")
			}
			if tc.input != tc.want && (!buffer.Undo() || buffer.String() != tc.input || buffer.CanUndo()) {
				t.Fatal("the operation was not one undo step")
			}
		})
	}
}

func TestSelectedLinesExcludeNextLineAtItsStart(t *testing.T) {
	const text = "keep\nb\n a \nuntouched\nlast"
	buffer := editor.NewBuffer(text)
	selection := editor.Selection{Anchor: 5, Head: 11}
	result, err := Prepare(context.Background(), buffer.Snapshot(), []editor.Selection{selection}, TrimBoth, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 || result.Edits[0].Start != 5 || result.Edits[0].End != 11 {
		t.Fatalf("wrong physical line scope: %+v", result.Edits)
	}
	buffer.Edit(result.Edits...)
	if buffer.String() != "keep\nb\na\nuntouched\nlast" {
		t.Fatalf("changed surrounding lines: %q", buffer.String())
	}
}

func TestDisjointSelectionsAndReversedUTF8Selection(t *testing.T) {
	const text = "b\na\nseparator\n😀\n中\n"
	buffer := editor.NewBuffer(text)
	sels := []editor.Selection{{Anchor: 0, Head: 4}, {Anchor: len(text), Head: 14}}
	result, err := Prepare(context.Background(), buffer.Snapshot(), sels, ReverseLines, 4)
	if err != nil {
		t.Fatal(err)
	}
	buffer.Edit(result.Edits...)
	if got := buffer.String(); got != "a\nb\nseparator\n中\n😀\n" {
		t.Fatalf("disjoint scopes mixed: %q", got)
	}
	if len(result.Selections) != 2 || result.Selections[1].Anchor < result.Selections[1].Head {
		t.Fatalf("selection direction lost: %+v", result.Selections)
	}
	for _, s := range result.Selections {
		if !utf8.ValidString(buffer.Slice(s.Start(), s.End())) {
			t.Fatalf("selection split a UTF-8 character: %+v", s)
		}
	}
}

func TestCaseConversionsMatchFramework(t *testing.T) {
	for _, tc := range []struct {
		op  Operation
		cmd editor.Command
	}{
		{Uppercase, editor.Uppercase}, {Lowercase, editor.Lowercase},
		{TitleCase, editor.TitleCase}, {SnakeCase, editor.SnakeCase},
		{KebabCase, editor.KebabCase}, {CamelCase, editor.CamelCase},
		{PascalCase, editor.PascalCase},
	} {
		for _, input := range []string{"hello HTTPServer foo-bar_baz", "éCOLE 中文😀 some_ID\nAnother line", "a.b URLValue x2Bar"} {
			expected := &editor.State{Buffer: editor.NewBuffer(input)}
			expected.Select(0, len(input))
			expected.Run(tc.cmd)
			actual := editor.NewBuffer(input)
			result, err := Prepare(context.Background(), actual.Snapshot(), []editor.Selection{{Head: len(input)}}, tc.op, 4)
			if err != nil {
				t.Fatal(err)
			}
			actual.Edit(result.Edits...)
			if actual.String() != expected.Buffer.String() {
				t.Fatalf("operation %d on %q: got %q want %q", tc.op, input, actual.String(), expected.Buffer.String())
			}
		}
	}
}

func TestCaseConversionsAffectExactSelectedBytes(t *testing.T) {
	buffer := editor.NewBuffer("before école after\n")
	result, err := Prepare(context.Background(), buffer.Snapshot(), []editor.Selection{{Anchor: 7, Head: 13}}, Uppercase, 4)
	if err != nil {
		t.Fatal(err)
	}
	buffer.Edit(result.Edits...)
	if got := buffer.String(); got != "before ÉCOLE after\n" {
		t.Fatalf("case conversion escaped selection: %q", got)
	}
}

func TestPartialLineSelectionCoversProcessedLines(t *testing.T) {
	buffer := editor.NewBuffer(" a \nuntouched\n")
	result, err := Prepare(context.Background(), buffer.Snapshot(), []editor.Selection{{Anchor: 2, Head: 1}}, TrimBoth, 4)
	if err != nil {
		t.Fatal(err)
	}
	buffer.Edit(result.Edits...)
	if len(result.Selections) != 1 || result.Selections[0] != (editor.Selection{Anchor: 2, Head: 0}) {
		t.Fatalf("reversed selection no longer covers the processed line: %+v", result.Selections)
	}
	if buffer.String() != "a\nuntouched\n" {
		t.Fatal("partial selection affected another line")
	}
}

func TestManyDisjointSelectionsKeepTheirOffsets(t *testing.T) {
	const count = 1000
	buffer := editor.NewBuffer(strings.Repeat(" é \nkeep\n", count))
	selections := make([]editor.Selection, count)
	for i := range selections {
		selections[i] = editor.Selection{Anchor: i * 10, Head: i*10 + 5}
	}
	result, err := Prepare(context.Background(), buffer.Snapshot(), selections, TrimBoth, 4)
	if err != nil {
		t.Fatal(err)
	}
	buffer.Edit(result.Edits...)
	if buffer.String() != strings.Repeat("é\nkeep\n", count) {
		t.Fatal("disjoint operation changed the unselected lines")
	}
	for i, s := range result.Selections {
		if s != (editor.Selection{Anchor: i * 8, Head: i*8 + 3}) {
			t.Fatalf("selection %d has incorrect translated offsets: %+v", i, s)
		}
	}
}

func TestNoOpCancellationAndInvalidOperation(t *testing.T) {
	for _, text := range []string{"", "a\nb\n", "中文😀"} {
		buffer := editor.NewBuffer(text)
		result, err := Prepare(context.Background(), buffer.Snapshot(), []editor.Selection{{}}, TrimTrailing, 4)
		if err != nil || len(result.Edits) != 0 || buffer.CanUndo() {
			t.Fatalf("no-op produced history: %+v %v", result, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Prepare(ctx, buffer.Snapshot(), nil, SortAscending, 4); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation ignored: %v", err)
		}
		if _, err := Prepare(context.Background(), buffer.Snapshot(), nil, 255, 4); err == nil {
			t.Fatal("unknown operation accepted")
		}
	}
}

func BenchmarkTrimSelectedLineIn100MB(b *testing.B) {
	snapshot := editor.NewBuffer(strings.Repeat("some text  \n", (100<<20)/12)).Snapshot()
	selection := []editor.Selection{{Anchor: 0, Head: 12}}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, err := Prepare(context.Background(), snapshot, selection, TrimTrailing, 4)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrimWhole100MB(b *testing.B) {
	snapshot := editor.NewBuffer(strings.Repeat("some text  \n", (100<<20)/12)).Snapshot()
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, err := Prepare(context.Background(), snapshot, nil, TrimTrailing, 4)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUniqueRepeated100MB(b *testing.B) {
	snapshot := editor.NewBuffer(strings.Repeat("a repeated line\n", (100<<20)/16)).Snapshot()
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		result, err := Prepare(context.Background(), snapshot, nil, UniqueLines, 4)
		if err != nil || len(result.Edits) != 1 || result.Edits[0].Text != "a repeated line\n" {
			b.Fatalf("incorrect deduplication: %+v %v", result, err)
		}
	}
}
