package search

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui/editor"
)

type Match struct{ Start, End int }

type Query struct {
	Text          string
	CaseSensitive bool
	Regex         bool
	Escapes       bool
}

type matcher struct {
	query     Query
	regex     *regexp.Regexp
	continued *regexp.Regexp
}

func compile(q Query, exact bool) (*matcher, error) {
	m := &matcher{query: q}
	if q.Regex {
		flags := "(?m)"
		if !q.CaseSensitive {
			flags = "(?im)"
		}
		anchor := ""
		if exact {
			anchor = `\A`
		}
		var err error
		m.regex, err = regexp.Compile(anchor + "(?:" + flags + q.Text + ")")
		if err != nil {
			return nil, locales.WrapError(err, locales.InvalidRegex)
		}
		// Consume the real preceding rune when restarting a reader. This keeps
		// ^, \A and word boundaries from treating a slice as a new document.
		m.continued, err = regexp.Compile(anchor + "(?s:.)(" + flags + q.Text + ")")
		if err != nil {
			return nil, locales.WrapError(err, locales.InvalidRegex)
		}
	} else if q.Escapes {
		text, err := unescape(q.Text)
		if err != nil {
			return nil, locales.WrapError(err, locales.InvalidQuery)
		}
		m.query.Text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	return m, nil
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

// Scan streams non-overlapping matches without copying the document or retaining
// all matches. Offsets are UTF-8 bytes in the editor's LF-normalized text.
func Scan(ctx context.Context, s editor.Snapshot, q Query, from, to int, found func(Match) bool) error {
	m, err := compile(q, false)
	if err != nil {
		return err
	}
	return m.scan(ctx, s, from, to, func(match Match, _ []int) bool { return found(match) })
}

func (m *matcher) scan(ctx context.Context, s editor.Snapshot, from, to int, found func(Match, []int) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	from, to = max(0, min(from, s.Len())), max(0, min(to, s.Len()))
	if from > to || m.query.Text == "" {
		return nil
	}
	if m.regex != nil {
		return m.scanRegex(ctx, s, from, to, found)
	}
	return scanLiteral(ctx, s, m.query, from, to, func(match Match) bool { return found(match, nil) })
}

// Literal searches keep the streaming KMP path, with memory proportional to the
// pattern rather than the file or its number of matches.
func scanLiteral(ctx context.Context, s editor.Snapshot, q Query, from, to int, found func(Match) bool) error {
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
	matcher, err := compile(q, false)
	if err != nil {
		return Match{}, false, err
	}
	at = max(0, min(at, s.Len()))
	var result Match
	ok := false
	scan := func(from, to int, excludeEmptyAtCursor bool) error {
		return matcher.scan(ctx, s, from, to, func(m Match, _ []int) bool {
			if excludeEmptyAtCursor && m.Start == at && m.End == at {
				return true
			}
			result, ok = m, true
			return backward
		})
	}
	if backward {
		if at > 0 {
			err = scan(0, at, true)
		}
		if !ok && err == nil {
			err = scan(at, s.Len(), false)
		}
	} else {
		err = scan(at, s.Len(), false)
		if !ok && err == nil && at > 0 {
			err = scan(0, at, false)
		}
	}
	return result, ok, err
}

// ReplaceAll writes unchanged chunks and replacements into a single result,
// instead of allocating an Edit for every occurrence.
func ReplaceAll(ctx context.Context, s editor.Snapshot, q Query, replacement string) (string, int, error) {
	matcher, err := compile(q, false)
	if err != nil {
		return "", 0, err
	}
	template, err := compileReplacement(q, matcher.regex, replacement)
	if err != nil {
		return "", 0, err
	}
	var out strings.Builder
	last, count := 0, 0
	var writeErr error
	projected := int64(s.Len())
	err = matcher.scan(ctx, s, 0, s.Len(), func(m Match, captures []int) bool {
		projected += template.size(captures) - int64(m.End-m.Start)
		if projected > maxResultSize {
			writeErr = errResultTooLarge
			return false
		}
		if count == 0 {
			out.Grow(min(s.Len(), 16<<20))
		}
		writeErr = writeRange(ctx, &out, s, last, m.Start)
		if writeErr == nil {
			writeErr = template.write(ctx, &out, s, captures)
		}
		if writeErr != nil {
			return false
		}
		last = m.End
		count++
		return ctx.Err() == nil
	})
	if err != nil {
		return "", 0, err
	}
	if writeErr != nil {
		return "", 0, writeErr
	}
	if count == 0 {
		return "", 0, nil
	}
	if err = writeRange(ctx, &out, s, last, s.Len()); err != nil {
		return "", 0, err
	}
	return out.String(), count, nil
}

const maxResultSize = 2 << 30

var errResultTooLarge = locales.NewError(locales.ReplacementTooLarge)

// ReplaceSelection expands a replacement only if the selection is an exact
// match in the original document, including its real anchor/boundary context.
func ReplaceSelection(ctx context.Context, s editor.Snapshot, q Query, replacement string, selection Match) (string, bool, error) {
	matcher, err := compile(q, true)
	if err != nil {
		return "", false, err
	}
	template, err := compileReplacement(q, matcher.regex, replacement)
	if err != nil {
		return "", false, err
	}
	var out strings.Builder
	exact := false
	var writeErr error
	err = matcher.scan(ctx, s, selection.Start, selection.End, func(m Match, captures []int) bool {
		if m != selection {
			return false
		}
		if int64(s.Len()-(m.End-m.Start))+template.size(captures) > maxResultSize {
			writeErr = errResultTooLarge
			return false
		}
		writeErr = template.write(ctx, &out, s, captures)
		exact = writeErr == nil
		return false
	})
	if err != nil {
		return "", false, err
	}
	if writeErr != nil {
		return "", false, writeErr
	}
	return out.String(), exact, nil
}

func writeRange(ctx context.Context, out *strings.Builder, s editor.Snapshot, from, to int) error {
	s.Chunks(from, to, func(chunk string) bool {
		if ctx.Err() != nil {
			return false
		}
		out.WriteString(chunk)
		return true
	})
	return ctx.Err()
}
