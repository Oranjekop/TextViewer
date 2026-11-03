package document

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/Oranjekop/TextViewer/locales"
	"github.com/egoist/mygo/ui/editor"
)

const (
	maxJSONInputBytes  = 128 << 20
	maxJSONOutputBytes = 256 << 20
)

// FormatJSON retains JSON lexemes (including numbers, escapes, duplicate keys
// and key order). Bound indentation expansion before allocating the output.
func FormatJSON(ctx context.Context, snapshot editor.Snapshot) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if snapshot.Len() > maxJSONInputBytes {
		return "", false, locales.NewError(locales.JsonInputTooLarge)
	}
	input := make([]byte, 0, snapshot.Len())
	snapshot.Chunks(0, snapshot.Len(), func(chunk string) bool {
		if ctx.Err() != nil {
			return false
		}
		input = append(input, chunk...)
		return true
	})
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !json.Valid(input) {
		var raw json.RawMessage
		err := json.Unmarshal(input, &raw)
		return "", false, locales.WrapError(err, locales.InvalidJSON)
	}
	size, err := jsonOutputBound(ctx, input)
	if err != nil {
		return "", false, err
	}
	output, err := indentJSON(ctx, input, size)
	if err != nil {
		return "", false, err
	}
	if bytes.Equal(input, []byte(output)) {
		return "", false, nil
	}
	return output, true, nil
}

// Input is already validated by encoding/json. Rewrite whitespace only, without
// the additional whole-value buffers used by Go's JSON v2 indentation path.
func indentJSON(ctx context.Context, input []byte, capacity int) (string, error) {
	end := len(input)
	for end > 0 && jsonSpace(input[end-1]) {
		end--
	}
	var output strings.Builder
	output.Grow(capacity)
	padding := strings.Repeat(" ", 20000)
	depth := 0
	quoted, escaped, needIndent := false, false, false
	newline := func() {
		output.WriteByte('\n')
		output.WriteString(padding[:depth*2])
	}
	for i, ch := range input[:end] {
		if i&16383 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if quoted {
			output.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
			continue
		}
		if jsonSpace(ch) {
			continue
		}
		if needIndent && ch != '}' && ch != ']' {
			needIndent = false
			depth++
			newline()
		}
		switch ch {
		case '{', '[':
			output.WriteByte(ch)
			needIndent = true
		case '}', ']':
			if needIndent {
				needIndent = false
			} else {
				depth--
				newline()
			}
			output.WriteByte(ch)
		case ',':
			output.WriteByte(ch)
			newline()
		case ':':
			output.WriteString(": ")
		default:
			output.WriteByte(ch)
			quoted = ch == '"'
		}
	}
	output.Write(input[end:])
	return output.String(), ctx.Err()
}

func jsonSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func jsonOutputBound(ctx context.Context, input []byte) (int, error) {
	size, depth := len(input), 0
	quoted, escaped := false, false
	for i, ch := range input {
		if i&16383 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			size += 1 + 2*depth
		case '}', ']':
			depth--
			size += 1 + 2*depth
		case ',':
			size += 1 + 2*depth
		case ':':
			size++
		}
		if size > maxJSONOutputBytes {
			return 0, locales.NewError(locales.JsonOutputTooLarge)
		}
	}
	return size, ctx.Err()
}
