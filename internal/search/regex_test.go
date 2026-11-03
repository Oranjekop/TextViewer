package search

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestRegexMatchesStandardLibraryAcrossChunksAndEmptyMatches(t *testing.T) {
	texts := []string{
		"", "a", "aaab", "abab", "foo bar\nFOO\n", "中文😀 中文\n字\n",
		"abc\n\n123\n", "word_word tail", "\r\n",
		strings.Repeat("a", 4094) + "中文😀\nfoo bar\n" + strings.Repeat("b", 4097),
	}
	patterns := []string{
		`^`, `$`, `^$`, `\A`, `\z`, `\b`, `\B`, `a*`, `a*?`, `a+|b`, `a|ab`,
		`(?:ab)?`, `[a-z]+`, `\d+`, `\n`, `\r?\n`, `中文|😀`, `(?s).+`, `(?s).*?`,
		`^foo\b`, `(?-m:^foo$)`, `(?P<word>\w+)([ \n]+)?`, `\A|foo|\z`,
	}
	for _, text := range texts {
		s := editor.NewBuffer(text).Snapshot()
		normalized := s.String()
		for _, pattern := range patterns {
			for _, sensitive := range []bool{false, true} {
				q := Query{Text: pattern, Regex: true, CaseSensitive: sensitive}
				flags := "(?im)"
				if sensitive {
					flags = "(?m)"
				}
				re := regexp.MustCompile(flags + pattern)
				var want []Match
				for _, indexes := range re.FindAllStringIndex(normalized, -1) {
					want = append(want, Match{indexes[0], indexes[1]})
				}
				var got []Match
				err := Scan(context.Background(), s, q, 0, s.Len(), func(match Match) bool {
					got = append(got, match)
					return len(got) <= len(want)+1
				})
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("pattern=%q sensitive=%v text length=%d: got %v err=%v want %v", pattern, sensitive, s.Len(), got, err, want)
				}
			}
		}
	}
}

func TestRegexReplacementMatchesStandardLibrary(t *testing.T) {
	texts := []string{"", "aaab ab", "foo bar\nFOO\n", "中文😀 字", "x=1\ny=22\n"}
	patterns := []string{
		`^`, `$`, `\b`, `a*`, `a*?`, `a|ab`, `(?P<word>\w+)([ \n]+)?`,
		`(a)(b)?`, `(?:(?P<same>a)|(?P<same>b))`, `(?P<name>[a-z]+)=(\d+)`, `(?s).+`,
	}
	templates := []string{
		"", "X", "$0", "$1/$2", "${word}:$2", "${name}=$2", "$$1", "$$$0",
		"$same", "$missing", "$01/${1}/$1x", "$", "${", "$-x", "$999999999999999999999999999",
		`$1\n\t${word}`,
	}
	for _, text := range texts {
		s := editor.NewBuffer(text).Snapshot()
		for _, pattern := range patterns {
			re := regexp.MustCompile("(?im)" + pattern)
			matches := re.FindAllStringIndex(text, -1)
			for _, template := range templates {
				got, count, err := ReplaceAll(context.Background(), s, Query{Text: pattern, Regex: true}, template)
				decoded, decodeErr := unescape(template)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				want := re.ReplaceAllString(text, decoded)
				if len(matches) == 0 {
					want = ""
				}
				if err != nil || count != len(matches) || got != want {
					t.Fatalf("text=%q pattern=%q replacement=%q: got %q count=%d err=%v; want %q count=%d", text, pattern, template, got, count, err, want, len(matches))
				}
			}
		}
	}
}

func TestEscapedSearchAndReplacement(t *testing.T) {
	s := editor.NewBuffer("one\ttwo\r\nthree\\four\rend\x00中😀").Snapshot()
	for _, tc := range []struct {
		text string
		want string
	}{
		{`\n`, "\n"}, {`\r\n`, "\n"}, {`\t`, "\t"}, {`\\`, `\`}, {`\r`, "\r"},
		{`\0`, "\x00"}, {`\x4f`, "o"}, {`\u4e2d`, "中"}, {`\U0001f600`, "😀"},
	} {
		q := Query{Text: tc.text, Escapes: true}
		match, ok, err := Next(context.Background(), s, q, 0, false)
		if err != nil || !ok || s.Slice(match.Start, match.End) != tc.want {
			t.Fatalf("%q: got %+v found=%v err=%v", tc.text, match, ok, err)
		}
	}
	for _, tc := range []struct {
		query       Query
		replacement string
		want        string
	}{
		{Query{Text: "x"}, `\n$1`, `\n$1\n$1`},
		{Query{Text: "x", Escapes: true}, `\n$1`, "\n$1\n$1"},
		{Query{Text: "x", Escapes: true}, `\\n`, `\n\n`},
		{Query{Text: "(x)", Regex: true}, `$1\n`, "x\nx\n"},
		{Query{Text: "x", Regex: true}, `\r\n`, "\n\n"},
		{Query{Text: "x", Regex: true}, `\u4e2d`, "中中"},
	} {
		text, count, err := ReplaceAll(context.Background(), editor.NewBuffer("xx").Snapshot(), tc.query, tc.replacement)
		if err != nil || count != 2 || text != tc.want {
			t.Fatalf("%+v: got %q count=%d err=%v", tc, text, count, err)
		}
	}
	literal := editor.NewBuffer(`literal\n actual` + "\n").Snapshot()
	match, ok, err := Next(context.Background(), literal, Query{Text: `\n`}, 0, false)
	if err != nil || !ok || match != (Match{7, 9}) {
		t.Fatalf("plain search interpreted an escape: %+v %v %v", match, ok, err)
	}
}

func TestReplaceSelectionUsesRealDocumentContext(t *testing.T) {
	s := editor.NewBuffer("foobar\nfoo bar\n").Snapshot()
	for _, tc := range []struct {
		pattern     string
		replacement string
		selection   Match
		want        string
		exact       bool
	}{
		{`foo$`, "X", Match{0, 3}, "", false},
		{`\bbar`, "X", Match{3, 6}, "", false},
		{`^foo`, "X", Match{7, 10}, "X", true},
		{`\Afoo`, "X", Match{7, 10}, "", false},
		{`\bbar$`, "X", Match{11, 14}, "X", true},
		{`(foo)(bar)`, "$2-$1", Match{0, 6}, "bar-foo", true},
		{`^`, `\t`, Match{7, 7}, "\t", true},
		{`$`, "X", Match{3, 3}, "", false},
		{`$`, "X", Match{14, 14}, "X", true},
	} {
		got, exact, err := ReplaceSelection(context.Background(), s, Query{Text: tc.pattern, Regex: true}, tc.replacement, tc.selection)
		if err != nil || exact != tc.exact || got != tc.want {
			t.Fatalf("%+v: got %q exact=%v err=%v", tc, got, exact, err)
		}
	}
}

func TestRegexNextPreviousWrapAndUnicodeOffsets(t *testing.T) {
	s := editor.NewBuffer("中文\n😀\nlast").Snapshot()
	for _, tc := range []struct {
		at       int
		backward bool
		want     int
	}{
		{0, false, 0}, {1, false, 7}, {7, false, 7}, {8, false, 12},
		{13, false, 0}, {12, true, 7}, {7, true, 0}, {0, true, 12},
	} {
		match, ok, err := Next(context.Background(), s, Query{Text: "^", Regex: true}, tc.at, tc.backward)
		if err != nil || !ok || match != (Match{tc.want, tc.want}) {
			t.Fatalf("%+v: got %+v found=%v err=%v", tc, match, ok, err)
		}
	}
	for _, text := range []string{"", "one line"} {
		s := editor.NewBuffer(text).Snapshot()
		for _, backward := range []bool{false, true} {
			match, ok, err := Next(context.Background(), s, Query{Text: "^", Regex: true}, 0, backward)
			if err != nil || !ok || match != (Match{0, 0}) {
				t.Fatalf("single empty match did not wrap: text=%q backward=%v match=%+v found=%v err=%v", text, backward, match, ok, err)
			}
		}
	}
}

func TestInvalidPatternsAndEscapesDoNotProducePartialEdits(t *testing.T) {
	s := editor.NewBuffer("original\ntext").Snapshot()
	for _, q := range []Query{
		{Text: "[", Regex: true}, {Text: "(?=x)", Regex: true}, {Text: `\1`, Regex: true},
		{Text: `\`, Escapes: true}, {Text: `\q`, Escapes: true}, {Text: `\uD800`, Escapes: true},
		{Text: `\UFFFFFFFF`, Escapes: true}, {Text: `\xA`, Escapes: true},
	} {
		if _, _, err := Next(context.Background(), s, q, 0, false); err == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
		text, count, err := ReplaceAll(context.Background(), s, q, "replacement")
		if err == nil || count != 0 || text != "" {
			t.Fatalf("invalid query produced results: %+v %q %d %v", q, text, count, err)
		}
	}
	for _, q := range []Query{{Text: "original", Escapes: true}, {Text: "original", Regex: true}} {
		if text, count, err := ReplaceAll(context.Background(), s, q, `\q`); err == nil || count != 0 || text != "" {
			t.Fatalf("invalid replacement produced results: %q %d %v", text, count, err)
		}
	}
	if s.String() != "original\ntext" {
		t.Fatal("validation changed the snapshot")
	}
}

type cancelDuringRead struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelDuringRead) Err() error {
	c.checks++
	if c.checks == 8 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestRegexCancellationDuringLongMatch(t *testing.T) {
	s := editor.NewBuffer(strings.Repeat("a\n", 1<<20)).Snapshot()
	for _, replacement := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		during := &cancelDuringRead{Context: ctx, cancel: cancel}
		q := Query{Text: "(?s).*missing", Regex: true}
		var err error
		if replacement {
			var text string
			var count int
			text, count, err = ReplaceAll(during, s, q, "X")
			if text != "" || count != 0 {
				t.Fatal("canceled replacement returned partial text")
			}
		} else {
			_, _, err = Next(during, s, q, 0, false)
		}
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("replacement=%v: %v", replacement, err)
		}
	}
}

func BenchmarkRegexFindMissing100MB(b *testing.B) {
	s := editor.NewBuffer(strings.Repeat("line of text 中文\n", (100<<20)/20)).Snapshot()
	b.ReportAllocs()
	b.SetBytes(int64(s.Len()))
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = Next(context.Background(), s, Query{Text: "not-in-the-file", Regex: true}, 0, false)
	}
}

func BenchmarkRegexReplaceMany(b *testing.B) {
	s := editor.NewBuffer(strings.Repeat("key=123\n", 1<<16)).Snapshot()
	b.ReportAllocs()
	b.SetBytes(int64(s.Len()))
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = ReplaceAll(context.Background(), s, Query{Text: `(\w+)=(\d+)`, Regex: true}, `$1:$2`)
	}
}
