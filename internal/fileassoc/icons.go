package fileassoc

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"

	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"github.com/Oranjekop/TextViewer/locales"
)

//go:embed icons/*.ico
var icons embed.FS

func writeIcons(dir string) error {
	for _, group := range filetypes.Groups() {
		data, err := icons.ReadFile("icons/" + group.IconName())
		if err != nil {
			return err
		}
		path := filepath.Join(dir, group.IconName())
		existing, err := os.ReadFile(path)
		if err == nil {
			if !bytes.Equal(existing, data) {
				return locales.NewError(func() string { return locales.FileIconMismatch(path) })
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			os.Remove(path)
			return writeErr
		}
		if closeErr != nil {
			os.Remove(path)
			return closeErr
		}
	}
	return nil
}
