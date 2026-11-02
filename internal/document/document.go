package document

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type Encoding string

const (
	UTF8    Encoding = "UTF-8"
	UTF8BOM Encoding = "UTF-8 BOM"
	UTF16LE Encoding = "UTF-16 LE"
	UTF16BE Encoding = "UTF-16 BE"
	GB18030 Encoding = "GB18030"
)

var ErrChanged = errors.New("文件已被其他程序修改，请另存为，或重新打开后再编辑")

type Document struct {
	Path     string
	Encoding Encoding
	Buffer   *editor.Buffer
	Saved    editor.Snapshot
	stamp    os.FileInfo
}

func New() *Document {
	b := editor.NewBuffer("")
	b.SetLineEnding("\r\n")
	return &Document{Encoding: UTF8, Buffer: b, Saved: b.Snapshot()}
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func Load(ctx context.Context, path string) (*Document, error) {
	path, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stamp, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stamp.Mode().IsRegular() {
		return nil, errors.New("请选择一个文本文件")
	}
	// Leave headroom for indexing, undo and decoding rather than risking an
	// unbounded allocation from a corrupt file or an unexpectedly large input.
	if stamp.Size() > 2<<30 {
		return nil, errors.New("当前版本支持打开不超过 2 GB 的文本文件")
	}
	r := bufio.NewReaderSize(contextReader{ctx, f}, 64<<10)
	prefix, _ := r.Peek(3)
	enc, skip := UTF8, 0
	switch {
	case len(prefix) >= 3 && string(prefix[:3]) == "\xef\xbb\xbf":
		enc, skip = UTF8BOM, 3
	case len(prefix) >= 2 && string(prefix[:2]) == "\xff\xfe":
		enc, skip = UTF16LE, 2
	case len(prefix) >= 2 && string(prefix[:2]) == "\xfe\xff":
		enc, skip = UTF16BE, 2
	}
	_, _ = r.Discard(skip)
	var source io.Reader = r
	if codec := codecFor(enc); codec != nil {
		source = transform.NewReader(source, codec.NewDecoder())
	}
	var text strings.Builder
	if stamp.Size() < 256<<20 {
		text.Grow(int(stamp.Size()))
	}
	if _, err = io.Copy(&text, source); err != nil {
		return nil, err
	}
	content := text.String()
	if enc == UTF8 && !utf8.ValidString(content) {
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		text = strings.Builder{}
		source = transform.NewReader(contextReader{ctx, f}, simplifiedchinese.GB18030.NewDecoder())
		if _, err = io.Copy(&text, source); err != nil {
			return nil, err
		}
		content, enc = text.String(), GB18030
	}
	// Some decoders repair malformed input instead of returning an error.
	// Validate that repair was not lossy, but allow a literal U+FFFD in text.
	if codec := codecFor(enc); codec != nil && strings.ContainsRune(content, utf8.RuneError) {
		if err = verifyRoundTrip(ctx, f, skip, codec, content); err != nil {
			return nil, err
		}
	}
	if !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		return nil, errors.New("这个文件不是可编辑的文本文件")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	endStamp, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stamp.Size() != endStamp.Size() || !stamp.ModTime().Equal(endStamp.ModTime()) {
		return nil, ErrChanged
	}
	b := editor.NewBuffer(content)
	return &Document{Path: path, Encoding: enc, Buffer: b, Saved: b.Snapshot(), stamp: stamp}, nil
}

func codecFor(enc Encoding) encoding.Encoding {
	switch enc {
	case UTF16LE:
		return textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM)
	case UTF16BE:
		return textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM)
	case GB18030:
		return simplifiedchinese.GB18030
	}
	return nil
}

func verifyRoundTrip(ctx context.Context, f *os.File, skip int, codec encoding.Encoding, text string) error {
	if _, err := f.Seek(int64(skip), io.SeekStart); err != nil {
		return err
	}
	encoded := transform.NewReader(strings.NewReader(text), codec.NewEncoder())
	actual, expected := make([]byte, 64<<10), make([]byte, 64<<10)
	invalid := errors.New("文件中有无法识别的编码内容，请先转换为 UTF-8")
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := encoded.Read(actual)
		if n > 0 {
			read, readErr := io.ReadFull(f, expected[:n])
			if readErr != nil || read != n || !bytes.Equal(actual[:n], expected[:n]) {
				return invalid
			}
		}
		if err == io.EOF {
			if n, readErr := f.Read(expected[:1]); n != 0 || readErr != io.EOF {
				return invalid
			}
			return nil
		}
		if err != nil {
			return invalid
		}
	}
}

func (d *Document) Dirty() bool {
	return d.Buffer.Version() != d.Saved.Version()
}

// HasChanges runs only when leaving a document. It compares chunks so undoing
// back to the saved text does not require two whole-document strings.
func (d *Document) HasChanges() bool {
	current := d.Buffer.Snapshot()
	if current.Version() == d.Saved.Version() {
		return false
	}
	if current.Len() != d.Saved.Len() {
		return true
	}
	at, changed := 0, false
	current.Chunks(0, current.Len(), func(chunk string) bool {
		if chunk != d.Saved.Slice(at, at+len(chunk)) {
			changed = true
			return false
		}
		at += len(chunk)
		return true
	})
	return changed
}

func (d *Document) DiskChanged() bool {
	if d.Path == "" || d.stamp == nil {
		return false
	}
	now, err := os.Stat(d.Path)
	return err != nil || !os.SameFile(now, d.stamp) || now.Size() != d.stamp.Size() || !now.ModTime().Equal(d.stamp.ModTime())
}

// SaveSnapshot writes the captured version to a sibling temporary file first.
// A failed conversion or write never truncates the user's existing document.
func (d *Document) SaveSnapshot(ctx context.Context, path string, snapshot editor.Snapshot) (os.FileInfo, error) {
	path, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	samePath := d.Path != "" && SamePath(path, d.Path)
	if samePath && d.DiskChanged() {
		return nil, ErrChanged
	}
	targetStamp, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	if targetStamp != nil && targetStamp.Mode().Perm()&0200 == 0 {
		return nil, errors.New("文件为只读，请使用另存为")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".textviewer-*.tmp")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()
	if targetStamp != nil {
		if err = tmp.Chmod(targetStamp.Mode().Perm()); err != nil {
			return nil, err
		}
	}
	var out io.Writer = tmp
	switch d.Encoding {
	case UTF8BOM:
		_, err = tmp.Write([]byte{0xef, 0xbb, 0xbf})
	case UTF16LE:
		_, err = tmp.Write([]byte{0xff, 0xfe})
	case UTF16BE:
		_, err = tmp.Write([]byte{0xfe, 0xff})
	}
	if err != nil {
		return nil, err
	}
	var encoded *transform.Writer
	if codec := codecFor(d.Encoding); codec != nil {
		encoded = transform.NewWriter(tmp, codec.NewEncoder())
		out = encoded
	}
	_, err = snapshot.WriteTo(contextWriter{ctx, out})
	if err == nil && encoded != nil {
		err = encoded.Close()
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if samePath && d.DiskChanged() {
		return nil, ErrChanged
	}
	latest, latestErr := os.Stat(path)
	if targetStamp == nil {
		if latestErr == nil {
			return nil, ErrChanged
		}
		if !os.IsNotExist(latestErr) {
			return nil, latestErr
		}
	} else if latestErr != nil || !os.SameFile(latest, targetStamp) || latest.Size() != targetStamp.Size() || !latest.ModTime().Equal(targetStamp.ModTime()) {
		return nil, ErrChanged
	}
	if err = replaceFile(tmpName, path); err != nil {
		return nil, fmt.Errorf("保存失败：%w", err)
	}
	return os.Stat(path)
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

func (d *Document) AcceptSave(path string, snapshot editor.Snapshot, stamp os.FileInfo) {
	if resolved, err := ResolvePath(path); err == nil {
		path = resolved
	}
	d.Path, d.Saved, d.stamp = path, snapshot, stamp
	if d.Buffer.Version() == snapshot.Version() {
		d.Buffer.MarkSaved()
	}
}

// ResolvePath also resolves directory junctions for new files. A Windows
// profile alias or symlink must not bypass the external-change check.
func ResolvePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(absolute)); err == nil {
		return filepath.Join(parent, filepath.Base(absolute)), nil
	}
	return absolute, nil
}
