package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestSearchUnicodeAndChunkBoundaries(t *testing.T) {
	text := strings.Repeat("a", 4094) + "中文😀 中文😀\nHello HELLO hello\nς Σ σ"
	s := editor.NewBuffer(text).Snapshot()
	for _, tc := range []struct {
		query Query
		count int
	}{
		{Query{Text: "中文😀"}, 2},
		{Query{Text: "hello"}, 3},
		{Query{Text: "Hello", CaseSensitive: true}, 1},
		{Query{Text: "Σ"}, 3},
		{Query{Text: "\nHello"}, 1},
		{Query{Text: ""}, 0},
		{Query{Text: "missing"}, 0},
	} {
		count := 0
		err := Scan(context.Background(), s, tc.query, 0, s.Len(), func(m Match) bool {
			count++
			if m.Start >= m.End || m.End > s.Len() {
				t.Fatalf("bad match %+v", m)
			}
			return true
		})
		if err != nil || count != tc.count {
			t.Fatalf("%+v: count=%d err=%v", tc.query, count, err)
		}
	}
}

func TestNextPreviousAndWrap(t *testing.T) {
	s := editor.NewBuffer("one x two x three x").Snapshot()
	q := Query{Text: "x"}
	for _, tc := range []struct {
		at       int
		backward bool
		want     int
	}{
		{0, false, 4}, {5, false, 10}, {11, false, 18}, {19, false, 4},
		{18, true, 10}, {10, true, 4}, {4, true, 18}, {0, true, 18},
	} {
		m, ok, err := Next(context.Background(), s, q, tc.at, tc.backward)
		if err != nil || !ok || m.Start != tc.want {
			t.Fatalf("%+v: got %+v %v %v", tc, m, ok, err)
		}
	}
}

func TestNonOverlappingReplacement(t *testing.T) {
	s := editor.NewBuffer("aaaaa 中文 中文").Snapshot()
	result, count, err := ReplaceAll(context.Background(), s, Query{Text: "aa"}, "X")
	if err != nil || count != 2 || result != "XXa 中文 中文" {
		t.Fatalf("got %q %d %v", result, count, err)
	}
	result, count, err = ReplaceAll(context.Background(), s, Query{Text: "中文"}, "")
	if err != nil || count != 2 || result != "aaaaa  " {
		t.Fatalf("got %q %d %v", result, count, err)
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := editor.NewBuffer(strings.Repeat("hello\n", 20000)).Snapshot()
	_, _, err := Next(ctx, s, Query{Text: "not found"}, 0, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	_, count, err := ReplaceAll(ctx, s, Query{Text: "hello"}, "world")
	if !errors.Is(err, context.Canceled) || count != 0 {
		t.Fatalf("got %d %v", count, err)
	}
}

func TestNoMatchesDoesNotAllocateWholeDocument(t *testing.T) {
	s := editor.NewBuffer(strings.Repeat("a", 2<<20)).Snapshot()
	allocs := testing.AllocsPerRun(3, func() {
		result, count, err := ReplaceAll(context.Background(), s, Query{Text: "b"}, "c")
		if err != nil || count != 0 || result != "" {
			t.Fatal("unexpected replacement")
		}
	})
	if allocs > 12 {
		t.Fatalf("no-match replacement allocates excessively: %.0f", allocs)
	}
}

func BenchmarkFindMissing100MB(b *testing.B) {
	s := editor.NewBuffer(strings.Repeat("line of text 中文\n", (100<<20)/20)).Snapshot()
	b.ReportAllocs()
	b.SetBytes(int64(s.Len()))
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = Next(context.Background(), s, Query{Text: "not-in-the-file"}, 0, false)
	}
}
