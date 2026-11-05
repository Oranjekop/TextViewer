package textops

import (
	"context"
	"encoding/base64"
	"strings"
	"unicode/utf8"

	"github.com/Oranjekop/TextViewer/locales"
)

const maxBase64Bytes = 32 << 20

func base64LimitError() error { return locales.NewError(locales.Base64TooLarge) }

func transformBase64(ctx context.Context, text string, op Operation) (string, error) {
	if len(text) > maxBase64Bytes {
		return "", base64LimitError()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if op == Base64Encode {
		var out strings.Builder
		out.Grow(base64.StdEncoding.EncodedLen(len(text)))
		encoder := base64.NewEncoder(base64.StdEncoding, &out)
		for len(text) > 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			n := min(len(text), 64<<10)
			if _, err := encoder.Write([]byte(text[:n])); err != nil {
				return "", err
			}
			text = text[n:]
		}
		if err := encoder.Close(); err != nil {
			return "", err
		}
		return out.String(), ctx.Err()
	}
	// Ignore only ASCII formatting whitespace, never arbitrary invalid bytes.
	var clean strings.Builder
	clean.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if i&0xffff == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		switch text[i] {
		case ' ', '\t', '\r', '\n':
		default:
			clean.WriteByte(text[i])
		}
	}
	input := clean.String()
	encoding := base64.StdEncoding
	if strings.ContainsAny(input, "-_") {
		encoding = base64.URLEncoding
	}
	if !strings.Contains(input, "=") {
		encoding = encoding.WithPadding(base64.NoPadding)
	}
	decoded, err := encoding.Strict().DecodeString(input)
	if err != nil {
		return "", locales.NewError(locales.InvalidBase64)
	}
	if !utf8.Valid(decoded) {
		return "", locales.NewError(locales.Base64NotText)
	}
	// Buffer.Edit normalizes CRLF too; normalize here so selection offsets
	// are calculated from the same text that the editor will store.
	return strings.ReplaceAll(string(decoded), "\r\n", "\n"), ctx.Err()
}
