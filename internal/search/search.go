package search

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type Match struct{ Start, End int }

type Query struct {
	Text          string
	CaseSensitive bool
}

func fold(r rune) rune {
	if r < utf8.RuneSelf {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	// SimpleFold includes equivalent forms such as both lowercase sigmas.
	minimum := r
	for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
		minimum = min(minimum, next)
	}
	return unicode.ToLower(minimum)
}

// Scan is a streaming KMP search. Only the pattern and its prefix table are
// retained, even when the file has millions of matches. Rope leaves end on
// UTF-8 boundaries; offsets remain original UTF-8 byte offsets.
func Scan(ctx context.Context, s editor.Snapshot, q Query, from, to int, found func(Match) bool) error {
	pattern := []rune(q.Text)
	if len(pattern) == 0 {
		return nil
	}
	if !q.CaseSensitive {
		for i := range pattern {
			pattern[i] = fold(pattern[i])
		}
	}
	table := make([]int, len(pattern))
	for i, prefix := 1, 0; i < len(pattern); i++ {
		for prefix > 0 && pattern[i] != pattern[prefix] {
			prefix = table[prefix-1]
		}
		if pattern[i] == pattern[prefix] {
			prefix++
		}
		table[i] = prefix
	}
	positions := make([]int, len(pattern))
	matched, seen, offset := 0, 0, from
	s.Chunks(from, to, func(chunk string) bool {
		if ctx.Err() != nil {
			return false
		}
		for local, r := range chunk {
			positions[seen%len(pattern)] = offset + local
			seen++
			original := r
			if !q.CaseSensitive {
				r = fold(r)
			}
			for matched > 0 && r != pattern[matched] {
				matched = table[matched-1]
			}
			if r == pattern[matched] {
				matched++
			}
			if matched == len(pattern) {
				m := Match{positions[(seen-len(pattern))%len(pattern)], offset + local + utf8.RuneLen(original)}
				if !found(m) {
					return false
				}
				matched = 0 // Replacements and navigation use non-overlapping matches.
			}
		}
		offset += len(chunk)
		return true
	})
	return ctx.Err()
}

func Next(ctx context.Context, s editor.Snapshot, q Query, at int, backward bool) (Match, bool, error) {
	at = max(0, min(at, s.Len()))
	var result Match
	ok := false
	scan := func(from, to int) error {
		return Scan(ctx, s, q, from, to, func(m Match) bool {
			result, ok = m, true
			return backward
		})
	}
	var err error
	if backward {
		err = scan(0, at)
		if !ok && err == nil {
			err = scan(at, s.Len())
		}
	} else {
		err = scan(at, s.Len())
		if !ok && err == nil {
			err = scan(0, at)
		}
	}
	return result, ok, err
}

// ReplaceAll writes unchanged chunks and replacements into a single result,
// instead of allocating an Edit for every occurrence.
func ReplaceAll(ctx context.Context, s editor.Snapshot, q Query, replacement string) (string, int, error) {
	var out strings.Builder
	last, count := 0, 0
	write := func(from, to int) {
		s.Chunks(from, to, func(chunk string) bool {
			_, _ = io.WriteString(&out, chunk)
			return true
		})
	}
	var sizeErr error
	projected := s.Len()
	err := Scan(ctx, s, q, 0, s.Len(), func(m Match) bool {
		projected += len(replacement) - (m.End - m.Start)
		if projected > 2<<30 {
			sizeErr = errors.New("替换后的文件超过 2 GB，请缩小替换范围")
			return false
		}
		if count == 0 {
			out.Grow(min(s.Len(), 16<<20))
		}
		write(last, m.Start)
		out.WriteString(replacement)
		last = m.End
		count++
		return ctx.Err() == nil
	})
	if err != nil {
		return "", 0, err
	}
	if sizeErr != nil {
		return "", 0, sizeErr
	}
	if count == 0 {
		return "", 0, nil
	}
	write(last, s.Len())
	return out.String(), count, ctx.Err()
}
