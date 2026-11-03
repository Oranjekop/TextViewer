package search

import (
	"context"
	"io"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type snapshotReader struct {
	ctx      context.Context
	snapshot editor.Snapshot
	chunk    string
	start    int
	offset   int
}

func (r *snapshotReader) ReadRune() (rune, int, error) {
	if r.offset == r.snapshot.Len() {
		return 0, 0, io.EOF
	}
	if r.offset < r.start || r.offset >= r.start+len(r.chunk) {
		if err := r.ctx.Err(); err != nil {
			return 0, 0, err
		}
		r.start = r.offset
		r.snapshot.Chunks(r.offset, r.snapshot.Len(), func(chunk string) bool {
			r.chunk = chunk
			return false
		})
	}
	value, size := utf8.DecodeRuneInString(r.chunk[r.offset-r.start:])
	r.offset += size
	return value, size, nil
}

func runeBefore(s editor.Snapshot, at int) int {
	_, size := utf8.DecodeLastRuneInString(s.Slice(max(0, at-utf8.UTFMax), at))
	return at - size
}

// Advance moves a byte offset past one complete rune, including at EOF for
// navigation wraparound. It never splits a Chinese character or emoji.
func Advance(s editor.Snapshot, at int) int {
	if at >= s.Len() {
		return s.Len() + 1
	}
	_, size := utf8.DecodeRuneInString(s.Slice(at, min(at+utf8.UTFMax, s.Len())))
	return at + size
}

func (m *matcher) scanRegex(ctx context.Context, s editor.Snapshot, from, to int, found func(Match, []int) bool) error {
	reader := &snapshotReader{ctx: ctx, snapshot: s}
	previousEnd := -1
	for at := from; at <= to; {
		if err := ctx.Err(); err != nil {
			return err
		}
		base, re := at, m.regex
		if at > 0 {
			base, re = runeBefore(s, at), m.continued
		}
		reader.offset = base
		captures := re.FindReaderSubmatchIndex(reader)
		if err := ctx.Err(); err != nil {
			return err
		}
		if captures == nil {
			return nil
		}
		if at > 0 {
			captures = captures[2:] // The wrapper capture is the real whole match.
		}
		for i := range captures {
			if captures[i] >= 0 {
				captures[i] += base
			}
		}
		match := Match{captures[0], captures[1]}
		if match.Start > to || match.End > to {
			return nil
		}
		// As with regexp.FindAll, ignore an empty match directly after a
		// nonempty match; always advance a full rune after an empty match.
		if match.Start != match.End || match.End != previousEnd {
			if !found(match, captures) {
				return ctx.Err()
			}
		}
		previousEnd = match.End
		at = match.End
		if match.Start == match.End {
			at = Advance(s, at)
		}
	}
	return ctx.Err()
}
