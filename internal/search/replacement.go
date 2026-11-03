package search

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui/editor"
)

func unescape(text string) (string, error) {
	if !strings.ContainsRune(text, '\\') {
		return text, nil
	}
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' {
			out.WriteByte(text[i])
			continue
		}
		i++
		if i == len(text) {
			return "", locales.NewError(locales.IncompleteEscape)
		}
		switch text[i] {
		case '\\':
			out.WriteByte('\\')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case '0':
			out.WriteByte(0)
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'v':
			out.WriteByte('\v')
		case 'x', 'u', 'U':
			digits := 2
			if text[i] == 'u' {
				digits = 4
			} else if text[i] == 'U' {
				digits = 8
			}
			if i+digits >= len(text) {
				return "", locales.NewError(func() string { return locales.HexEscapeDigits(string(text[i]), strconv.Itoa(digits)) })
			}
			value, err := strconv.ParseUint(text[i+1:i+1+digits], 16, 32)
			if err != nil || !utf8.ValidRune(rune(value)) {
				return "", locales.NewError(func() string { return locales.InvalidEscape(text[i : i+1+digits]) })
			}
			out.WriteRune(rune(value))
			i += digits
		default:
			return "", locales.NewError(func() string { return locales.UnsupportedEscape(string(text[i])) })
		}
	}
	return out.String(), nil
}

type replacementPart struct {
	literal string
	groups  []int
}

type replacementTemplate []replacementPart

func compileReplacement(q Query, re *regexp.Regexp, text string) (replacementTemplate, error) {
	if q.Regex || q.Escapes {
		var err error
		text, err = unescape(text)
		if err != nil {
			return nil, locales.WrapError(err, locales.InvalidReplacement)
		}
	}
	// The editor stores LF internally and preserves the chosen output EOL.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if re == nil {
		return replacementTemplate{{literal: text}}, nil
	}
	var parts replacementTemplate
	for text != "" {
		before, after, hasDollar := strings.Cut(text, "$")
		if before != "" {
			parts = append(parts, replacementPart{literal: before})
		}
		if !hasDollar {
			break
		}
		text = after
		if strings.HasPrefix(text, "$") {
			parts = append(parts, replacementPart{literal: "$"})
			text = text[1:]
			continue
		}
		name, rest, ok := groupName(text)
		if !ok {
			parts = append(parts, replacementPart{literal: "$"})
			continue
		}
		text = rest
		part := replacementPart{}
		number, err := strconv.Atoi(name)
		if err == nil && number >= 0 && number <= re.NumSubexp() && (name == "0" || name[0] != '0') {
			part.groups = []int{number}
		} else {
			for index, candidate := range re.SubexpNames() {
				if candidate == name {
					part.groups = append(part.groups, index)
				}
			}
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func groupName(text string) (string, string, bool) {
	braced := strings.HasPrefix(text, "{")
	value := text
	if braced {
		value = value[1:]
	}
	end := 0
	for _, r := range value {
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		end += utf8.RuneLen(r)
	}
	if end == 0 || (braced && (end == len(value) || value[end] != '}')) {
		return "", text, false
	}
	name := value[:end]
	if braced {
		end++
	}
	return name, value[end:], true
}

func (part replacementPart) capture(captures []int) (int, int, bool) {
	for _, group := range part.groups {
		if 2*group+1 < len(captures) && captures[2*group] >= 0 {
			return captures[2*group], captures[2*group+1], true
		}
	}
	return 0, 0, false
}

func (parts replacementTemplate) size(captures []int) int64 {
	var size int64
	for _, part := range parts {
		size += int64(len(part.literal))
		if from, to, ok := part.capture(captures); ok {
			size += int64(to - from)
		}
	}
	return size
}

func (parts replacementTemplate) write(ctx context.Context, out *strings.Builder, s editor.Snapshot, captures []int) error {
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		out.WriteString(part.literal)
		if from, to, ok := part.capture(captures); ok {
			if err := writeRange(ctx, out, s, from, to); err != nil {
				return err
			}
		}
	}
	return nil
}
