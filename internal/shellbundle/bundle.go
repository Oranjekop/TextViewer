// Package shellbundle stores the Explorer integration in the executable's
// self-extracting ZIP. Reading it is only necessary when configuring Windows.
package shellbundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Oranjekop/TextViewer/locales"
	"io"
	"os"
	"path/filepath"
)

var Files = []string{"TextViewerShell.dll", "TextViewer.ContextMenu.msix", "TextViewer.ContextMenu.cer", "ContextMenu.ps1"}

// Read also works for foreign-architecture EXEs, allowing release validation
// without executing an ARM64 program on an x64 build machine.
func Read(executable string) (map[string][]byte, error) {
	reader, err := zip.OpenReader(executable)
	if err != nil {
		return nil, locales.WrapError(err, locales.MissingBundle)
	}
	defer reader.Close()
	files := make(map[string][]byte, len(Files))
	for _, name := range Files {
		var entry *zip.File
		for _, candidate := range reader.File {
			if candidate.Name == name {
				if entry != nil {
					return nil, fmt.Errorf("duplicate embedded file %s", name)
				}
				entry = candidate
			}
		}
		if entry == nil || entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > 16<<20 {
			return nil, fmt.Errorf("invalid embedded file %s", name)
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(stream, (16<<20)+1))
		stream.Close()
		if err != nil || len(data) > 16<<20 {
			return nil, fmt.Errorf("read embedded %s: %w", name, err)
		}
		files[name] = data
	}
	return files, nil
}

// Append runs before Authenticode signing and installer/archive creation.
// ZIP offsets are relative to its own start, as supported by self-extracting ZIP
// readers. Nothing is unpacked by the Windows loader or the editor's startup.
func Append(executable, resources string) (err error) {
	binary, err := pe.Open(executable)
	if err != nil {
		return err
	}
	header, ok := binary.OptionalHeader.(*pe.OptionalHeader64)
	binary.Close()
	if !ok || header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY].Size != 0 {
		return errors.New("embed Windows integration before signing the executable")
	}
	if _, err := Read(executable); err == nil {
		return errors.New("executable already contains Windows integration; rebuild it first")
	}
	source, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer source.Close()
	file, err := os.CreateTemp(filepath.Dir(executable), ".textviewer-bundle-*.exe")
	if err != nil {
		return err
	}
	defer func() { file.Close(); os.Remove(file.Name()) }()
	if _, err = io.Copy(file, source); err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	for _, name := range Files {
		data, err := os.ReadFile(filepath.Join(resources, name))
		if err != nil || len(data) == 0 || len(data) > 16<<20 {
			archive.Close()
			return fmt.Errorf("missing or invalid integration resource %s", name)
		}
		entry, err := archive.Create(name)
		if err == nil {
			_, err = entry.Write(data)
		}
		if err != nil {
			archive.Close()
			return err
		}
	}
	if err = archive.Close(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if _, err = Read(file.Name()); err != nil {
		return err
	}
	source.Close()
	return os.Rename(file.Name(), executable)
}

func Directory(executable, root string) (string, error) {
	file, err := os.Open(executable)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return filepath.Join(root, "app-"+hex.EncodeToString(hash.Sum(nil))[:16]), nil
}

// Prepare publishes a complete immutable directory atomically. Concurrent
// instances either publish their staging directory or verify the winner.
func Prepare(executable, root string) (string, error) {
	files, err := Read(executable)
	if err != nil {
		return "", err
	}
	directory, err := Directory(executable, root)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if _, err = os.Lstat(directory); err == nil {
		return directory, verify(directory, executable, files)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	staging, err := os.MkdirTemp(root, ".prepare-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(staging, name), data, 0600); err != nil {
			return "", err
		}
	}
	source, err := os.Open(executable)
	if err != nil {
		return "", err
	}
	target, err := os.Create(filepath.Join(staging, "TextViewer.exe"))
	if err != nil {
		source.Close()
		return "", err
	}
	_, err = io.Copy(target, source)
	source.Close()
	closeErr := target.Close()
	if err != nil || closeErr != nil {
		return "", errors.Join(err, closeErr)
	}
	if err = os.Rename(staging, directory); err != nil {
		if check := verify(directory, executable, files); check != nil {
			return "", errors.Join(err, check)
		}
	}
	return directory, nil
}

func verify(directory, executable string, files map[string][]byte) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("integration directory must not be a link")
	}
	for _, name := range append([]string{"TextViewer.exe"}, Files...) {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid immutable integration file %s", name)
		}
		if name == "TextViewer.exe" {
			actual, err := Directory(path, filepath.Dir(directory))
			if err != nil || actual != directory {
				return fmt.Errorf("immutable executable has changed: %s", path)
			}
		} else {
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, files[name]) {
				return fmt.Errorf("immutable integration file has changed: %s", path)
			}
		}
	}
	return nil
}
