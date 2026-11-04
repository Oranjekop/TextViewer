package desktop

import (
	"bytes"
	"image"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

const (
	previewImageBytes  = 8 << 20
	previewImagePixels = 8 << 20
	previewImageCount  = 16
)

func previewFileImages(directory string) func(string) (ui.ImageSource, error) {
	var mu sync.Mutex
	var count, pixels int
	workers := make(chan struct{}, 2)
	return func(address string) (ui.ImageSource, error) {
		path, err := previewImagePath(directory, address)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		if count >= previewImageCount {
			mu.Unlock()
			return nil, os.ErrInvalid
		}
		count++
		mu.Unlock()
		workers <- struct{}{}
		defer func() { <-workers }()
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > previewImageBytes {
			return nil, os.ErrInvalid
		}
		data, err := io.ReadAll(io.LimitReader(file, previewImageBytes+1))
		if err != nil || len(data) > previewImageBytes {
			return nil, os.ErrInvalid
		}
		reserve := func(area int64) bool {
			mu.Lock()
			defer mu.Unlock()
			if area <= 0 || area > int64(previewImagePixels-pixels) {
				return false
			}
			pixels += int(area)
			return true
		}
		if strings.EqualFold(filepath.Ext(path), ".svg") {
			if len(data) > 128<<10 {
				return nil, os.ErrInvalid
			}
			svg, err := ui.ParseSVG(data)
			if err != nil {
				return nil, err
			}
			w, h := svg.Size()
			if w <= 0 || h <= 0 || w > previewImagePixels || h > previewImagePixels ||
				!reserve(int64(w)*int64(h)) {
				return nil, os.ErrInvalid
			}
			return svg, nil
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if !reserve(int64(config.Width) * int64(config.Height)) {
			return nil, os.ErrInvalid
		}
		return ui.DecodeBitmap(data)
	}
}

func previewImagePath(directory, address string) (string, error) {
	path := address
	if !filepath.IsAbs(path) {
		u, err := url.Parse(address)
		if err != nil || u.Host != "" || u.Scheme != "" && u.Scheme != "file" || u.Path == "" {
			return "", os.ErrInvalid
		}
		path = u.Path
		if len(path) > 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(directory, path)
	}
	if strings.HasPrefix(filepath.VolumeName(path), `\\`) {
		return "", os.ErrInvalid
	}
	return filepath.Clean(path), nil
}
