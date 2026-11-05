// macbundle adds the shared file catalog and high-resolution document icons to
// MyGo's Universal bundle before its final signature is applied.
package main

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"golang.org/x/image/draw"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: macbundle path/to/TextViewer.app")
		os.Exit(1)
	}
	if err := prepare(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare(bundle string) error {
	executable := filepath.Join(bundle, "Contents", "MacOS", "TextViewer")
	fat, err := macho.OpenFat(executable)
	if err != nil {
		return fmt.Errorf("Universal executable: %w", err)
	}
	defer fat.Close()
	architectures := map[macho.Cpu]bool{}
	for _, arch := range fat.Arches {
		architectures[arch.Cpu] = true
	}
	if len(fat.Arches) != 2 || !architectures[macho.CpuArm64] || !architectures[macho.CpuAmd64] {
		return fmt.Errorf("bundle must contain exactly arm64 and x86_64")
	}
	path := filepath.Join(bundle, "Contents", "Info.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.Contains(data, []byte("<key>CFBundleDocumentTypes</key>")) {
		return fmt.Errorf("bundle already has document types; rebuild before preparing")
	}
	resources := filepath.Join(bundle, "Contents", "Resources")
	var docs strings.Builder
	docs.WriteString("<key>CFBundleDocumentTypes</key><array>")
	for _, group := range filetypes.Groups() {
		file, err := os.Open(filepath.Join("assets", "file-icons", group.ID+"-master.png"))
		if err != nil {
			return err
		}
		master, err := png.Decode(file)
		file.Close()
		if err != nil {
			return err
		}
		icon, err := encodeICNS(master)
		if err != nil {
			return err
		}
		name := "FileIcon-" + group.ID + ".icns"
		if err := os.WriteFile(filepath.Join(resources, name), icon, 0644); err != nil {
			return err
		}
		docs.WriteString("<dict><key>CFBundleTypeName</key><string>")
		xml.EscapeText(&docs, []byte(group.Name))
		docs.WriteString("</string><key>CFBundleTypeRole</key><string>Editor</string><key>LSHandlerRank</key><string>Alternate</string><key>CFBundleTypeIconFile</key><string>")
		docs.WriteString(name)
		docs.WriteString("</string><key>CFBundleTypeExtensions</key><array>")
		for _, ext := range group.Extensions {
			docs.WriteString("<string>" + strings.TrimPrefix(ext, ".") + "</string>")
		}
		docs.WriteString("</array></dict>")
	}
	docs.WriteString("</array>")
	index := bytes.LastIndex(data, []byte("</dict>"))
	if index < 0 {
		return fmt.Errorf("missing bundle property dictionary")
	}
	data = []byte(string(data[:index]) + docs.String() + string(data[index:]))
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := decoder.Token(); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("invalid bundle property list: %w", err)
		}
	}
	return os.WriteFile(path, data, 0644)
}

func encodeICNS(master image.Image) ([]byte, error) {
	var chunks bytes.Buffer
	for _, size := range []struct {
		tag    string
		pixels int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}} {
		img := image.NewNRGBA(image.Rect(0, 0, size.pixels, size.pixels))
		draw.CatmullRom.Scale(img, img.Bounds(), master, master.Bounds(), draw.Src, nil)
		var pngData bytes.Buffer
		if err := png.Encode(&pngData, img); err != nil {
			return nil, err
		}
		chunks.WriteString(size.tag)
		binary.Write(&chunks, binary.BigEndian, uint32(pngData.Len()+8))
		chunks.Write(pngData.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(chunks.Len()+8))
	out.Write(chunks.Bytes())
	return out.Bytes(), nil
}
