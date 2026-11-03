package textops

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type Operation uint8

const (
	SortAscending Operation = iota
	SortDescending
	SortIgnoreCase
	ReverseLines
	UniqueLines
	TrimLeading
	TrimTrailing
	TrimBoth
	RemoveEmptyLines
	RemoveBlankLines
	TabsToSpaces
	LeadingSpacesToTabs
	Uppercase
	Lowercase
	TitleCase
	InvertCase
	SnakeCase
	KebabCase
	CamelCase
	PascalCase
)

type span struct{ start, end int }

type Result struct {
	Edits      []editor.Edit
	Selections []editor.Selection
}

// Prepare works on immutable text, without touching the live editor. Line
// operations cover selected physical lines, or the document when none is selected.
// Case conversions work on the exact selections.
func Prepare(ctx context.Context, snapshot editor.Snapshot, selections []editor.Selection, op Operation, tabSize int) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if op > PascalCase {
		return Result{}, fmt.Errorf("unknown text operation %d", op)
	}
	if tabSize <= 0 {
		tabSize = 4
	}
	var spans []span
	selected := slices.Clone(selections)
	hasSelection := slices.ContainsFunc(selections, func(s editor.Selection) bool { return !s.Empty() })
	for i, s := range selections {
		if !hasSelection {
			spans = []span{{0, snapshot.Len()}}
			break
		}
		if s.Empty() {
			continue
		}
		if op >= Uppercase {
			spans = append(spans, span{s.Start(), s.End()})
			continue
		}
		first, last := snapshot.LineOf(s.Start()), snapshot.LineOf(s.End())
		if last > first && s.End() == snapshot.LineStart(last) {
			last--
		}
		start, end := snapshot.LineStart(first), min(snapshot.Len(), snapshot.LineStart(last+1))
		spans = append(spans, span{start, end})
		if s.Anchor > s.Head {
			start, end = end, start
		}
		selected[i] = editor.Selection{Anchor: start, Head: end}
	}
	if len(selections) == 0 {
		spans = []span{{0, snapshot.Len()}}
	}
	slices.SortFunc(spans, func(a, b span) int { return a.start - b.start })
	merged := spans[:0]
	for _, s := range spans {
		if len(merged) > 0 && (s.start < merged[len(merged)-1].end || op < Uppercase && s.start == merged[len(merged)-1].end) {
			merged[len(merged)-1].end = max(merged[len(merged)-1].end, s.end)
		} else {
			merged = append(merged, s)
		}
	}
	result := Result{Selections: selected}
	for _, s := range merged {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		input := snapshot.Slice(s.start, s.end)
		output, err := transform(ctx, input, op, tabSize)
		if err != nil {
			return Result{}, err
		}
		if output != input {
			result.Edits = append(result.Edits, editor.Edit{Start: s.start, End: s.end, Text: output})
		}
	}
	if len(result.Edits) == 0 {
		result.Selections = slices.Clone(selections)
		return result, ctx.Err()
	}
	deltas := make([]int, len(result.Edits)+1)
	for i, e := range result.Edits {
		deltas[i+1] = deltas[i] + len(e.Text) - (e.End - e.Start)
	}
	for i, s := range result.Selections {
		if s.Empty() {
			at := mapOffset(s.Head, result.Edits, deltas, false)
			result.Selections[i] = editor.Selection{Anchor: at, Head: at}
		} else {
			start, end := mapOffset(s.Start(), result.Edits, deltas, false), mapOffset(s.End(), result.Edits, deltas, true)
			if s.Anchor > s.Head {
				start, end = end, start
			}
			result.Selections[i] = editor.Selection{Anchor: start, Head: end}
		}
	}
	return result, ctx.Err()
}

func mapOffset(at int, edits []editor.Edit, deltas []int, right bool) int {
	index := sort.Search(len(edits), func(i int) bool { return edits[i].End >= at })
	if index < len(edits) && at >= edits[index].Start {
		e := edits[index]
		relative := min(at-e.Start, len(e.Text))
		if right || at == e.End {
			relative = len(e.Text)
		}
		for relative > 0 && relative < len(e.Text) && !utf8.RuneStart(e.Text[relative]) {
			relative--
		}
		return e.Start + deltas[index] + relative
	}
	return at + deltas[index]
}

func transform(ctx context.Context, text string, op Operation, tabSize int) (string, error) {
	if text == "" {
		return "", nil
	}
	if op >= Uppercase {
		var output string
		switch op {
		case Uppercase:
			output = strings.ToUpper(text)
		case Lowercase:
			output = strings.ToLower(text)
		case TitleCase:
			previousWord := false
			output = strings.Map(func(ch rune) rune {
				title := !previousWord
				previousWord = ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
				if title {
					return unicode.ToTitle(ch)
				}
				return unicode.ToLower(ch)
			}, text)
		case InvertCase:
			output = strings.Map(func(ch rune) rune {
				if unicode.IsUpper(ch) {
					return unicode.ToLower(ch)
				}
				return unicode.ToUpper(ch)
			}, text)
		case SnakeCase, KebabCase, CamelCase, PascalCase:
			return convertName(ctx, text, op)
		}
		return output, ctx.Err()
	}
	terminated := strings.HasSuffix(text, "\n")
	body := text
	if terminated {
		body = text[:len(text)-1]
	}
	if op == UniqueLines {
		seen := make(map[string]struct{})
		var lines []string
		for {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			line, rest, found := strings.Cut(body, "\n")
			if _, exists := seen[line]; !exists {
				seen[line] = struct{}{}
				lines = append(lines, line)
			}
			if !found {
				break
			}
			body = rest
		}
		return joinLines(ctx, lines, terminated)
	}
	if op <= UniqueLines {
		lines := strings.Split(body, "\n")
		switch op {
		case SortAscending:
			slices.Sort(lines)
		case SortDescending:
			slices.SortFunc(lines, func(a, b string) int { return strings.Compare(b, a) })
		case SortIgnoreCase:
			type keyedLine struct{ text, key string }
			keyed := make([]keyedLine, len(lines))
			for i, line := range lines {
				if i%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return "", err
					}
				}
				keyed[i] = keyedLine{line, strings.ToLower(line)}
			}
			slices.SortStableFunc(keyed, func(a, b keyedLine) int { return strings.Compare(a.key, b.key) })
			for i, line := range keyed {
				lines[i] = line.text
			}
		case ReverseLines:
			slices.Reverse(lines)
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return joinLines(ctx, lines, terminated)
	}
	var output strings.Builder
	output.Grow(len(text))
	for offset := 0; offset < len(text); {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		end := strings.IndexByte(text[offset:], '\n')
		newline := end >= 0
		if newline {
			end += offset
		} else {
			end = len(text)
		}
		line := text[offset:end]
		switch op {
		case RemoveEmptyLines, RemoveBlankLines:
			empty := line == ""
			if op == RemoveBlankLines {
				empty = strings.Trim(line, " \t") == ""
			}
			if empty {
				offset = end + 1
				continue
			}
		case TrimLeading:
			line = strings.TrimLeft(line, " \t")
		case TrimTrailing:
			line = strings.TrimRight(line, " \t")
		case TrimBoth:
			line = strings.Trim(line, " \t")
		}
		if op == TabsToSpaces || op == LeadingSpacesToTabs {
			if err := writeIndent(ctx, &output, line, op, tabSize); err != nil {
				return "", err
			}
		} else {
			output.WriteString(line)
		}
		if newline {
			output.WriteByte('\n')
		}
		offset = end + 1
	}
	return output.String(), ctx.Err()
}

func joinLines(ctx context.Context, lines []string, terminated bool) (string, error) {
	size := max(0, len(lines)-1)
	if terminated {
		size++
	}
	for _, line := range lines {
		size += len(line)
	}
	var output strings.Builder
	output.Grow(size)
	for i, line := range lines {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		output.WriteString(line)
		if i+1 < len(lines) || terminated {
			output.WriteByte('\n')
		}
	}
	return output.String(), ctx.Err()
}

func convertName(ctx context.Context, text string, op Operation) (string, error) {
	var output strings.Builder
	output.Grow(len(text))
	words := 0
	writeWord := func(word string) {
		if word == "" {
			return
		}
		if words > 0 {
			if op == SnakeCase {
				output.WriteByte('_')
			} else if op == KebabCase {
				output.WriteByte('-')
			}
		}
		first := true
		for _, ch := range word {
			ch = unicode.ToLower(ch)
			if first && (op == PascalCase || op == CamelCase && words > 0) {
				ch = unicode.ToTitle(ch)
			}
			output.WriteRune(ch)
			first = false
		}
		words++
	}
	start := 0
	var previous rune
	for at, ch := range text {
		if at%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		if ch == '_' || ch == '-' || unicode.IsSpace(ch) {
			writeWord(text[start:at])
			start = at + size
		} else if at > start && unicode.IsUpper(ch) {
			next, _ := utf8.DecodeRuneInString(text[at+size:])
			if unicode.IsLower(previous) || unicode.IsUpper(previous) && unicode.IsLower(next) {
				writeWord(text[start:at])
				start = at
			}
		}
		previous = ch
	}
	writeWord(text[start:])
	return output.String(), ctx.Err()
}

func writeIndent(ctx context.Context, output *strings.Builder, line string, op Operation, tabSize int) error {
	if op == LeadingSpacesToTabs {
		at, column := 0, 0
		for at < len(line) && (line[at] == ' ' || line[at] == '\t') {
			if at%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if line[at] == '\t' {
				column += tabSize - column%tabSize
			} else {
				column++
			}
			at++
		}
		output.WriteString(strings.Repeat("\t", column/tabSize))
		output.WriteString(strings.Repeat(" ", column%tabSize))
		output.WriteString(line[at:])
		return ctx.Err()
	}
	spaces := strings.Repeat(" ", tabSize)
	column := 0
	for at := 0; at < len(line); {
		if at%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		ch, size := utf8.DecodeRuneInString(line[at:])
		switch ch {
		case '\t':
			width := tabSize - column%tabSize
			output.WriteString(spaces[:width])
			column += width
		default:
			output.WriteString(line[at : at+size])
			column++
		}
		at += size
	}
	return ctx.Err()
}
