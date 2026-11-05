package desktop

import (
	"context"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type characterResult struct {
	count int
	err   error
}

type characterCounter struct {
	snapshot editor.Snapshot
	seen     bool
	ready    bool
	count    int
	due      time.Time
	cancel   context.CancelFunc
	result   <-chan characterResult
}

func countCharacters(ctx context.Context, snapshot editor.Snapshot) (int, error) {
	count := 0
	snapshot.Chunks(0, snapshot.Len(), func(text string) bool {
		if ctx.Err() != nil {
			return false
		}
		count += utf8.RuneCountInString(text)
		return true
	})
	return count, ctx.Err()
}

func (s *characterCounter) release() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.result = nil, nil
}

func (s *characterCounter) text(c *ui.Context, snapshot editor.Snapshot) string {
	if !s.seen || s.snapshot != snapshot {
		changed := s.seen
		s.release()
		s.snapshot, s.seen, s.ready = snapshot, true, false
		s.due = c.Now()
		if changed {
			s.due = s.due.Add(200 * time.Millisecond)
		}
	}
	if !s.ready {
		if snapshot.Len() <= 64<<10 {
			s.count, _ = countCharacters(context.Background(), snapshot)
			s.ready = true
		} else if remaining := s.due.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
		} else if s.result == nil {
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan characterResult, 1)
			s.cancel, s.result = cancel, result
			go func() {
				count, err := countCharacters(ctx, snapshot)
				result <- characterResult{count, err}
			}()
		}
		if s.result != nil {
			select {
			case result := <-s.result:
				s.release()
				s.count, s.ready = result.count, result.err == nil
			default:
				c.After(20 * time.Millisecond)
			}
		}
	}
	if !s.ready {
		return "…"
	}
	return strconv.Itoa(s.count)
}
