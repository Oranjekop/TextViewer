package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Oranjekop/TextViewer/internal/filetypes"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func main() {
	dir := "assets"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := generateAssets(dir); err != nil {
		log.Fatal(err)
	}
	if err := generateFileIcons(filepath.Join(dir, "file-icons"), filepath.Join("internal", "fileassoc", "icons")); err != nil {
		log.Fatal(err)
	}
	if err := generateIconPreview(filepath.Join(dir, "file-icons")); err != nil {
		log.Fatal(err)
	}
}

var iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}

func generateFileIcons(sourceDir, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	for _, group := range filetypes.Groups() {
		path := filepath.Join(sourceDir, group.ID+"-master.png")
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		master, err := png.Decode(source)
		source.Close()
		if err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		bounds := master.Bounds()
		if bounds.Dx() != bounds.Dy() || bounds.Dx() < 256 {
			return fmt.Errorf("%s must be square and at least 256 pixels", path)
		}
		data, err := encodeIcon(master)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outputDir, group.IconName()), data, 0644); err != nil {
			return err
		}
		thumbnail := image.NewNRGBA(image.Rect(0, 0, 256, 256))
		draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), master, bounds, draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, thumbnail); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(sourceDir, group.ID+".png"), encoded.Bytes(), 0644); err != nil {
			return err
		}
	}
	return nil
}

func encodeIcon(master image.Image) ([]byte, error) {
	images := make([][]byte, len(iconSizes))
	for i, size := range iconSizes {
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(img, img.Bounds(), master, master.Bounds(), draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			return nil, err
		}
		images[i] = encoded.Bytes()
	}
	var data bytes.Buffer
	binary.Write(&data, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	offset := uint32(6 + 16*len(images))
	for i, pixels := range images {
		size := byte(iconSizes[i]) // ICO uses zero for 256.
		data.Write([]byte{size, size, 0, 0})
		binary.Write(&data, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&data, binary.LittleEndian, [2]uint32{uint32(len(pixels)), offset})
		offset += uint32(len(pixels))
	}
	for _, pixels := range images {
		data.Write(pixels)
	}
	return data.Bytes(), nil
}

func generateIconPreview(dir string) error {
	const cell, rowHeight = 180, 236
	groups := filetypes.Groups()
	preview := image.NewNRGBA(image.Rect(0, 0, cell*len(groups), rowHeight*2))
	for row, background := range []color.NRGBA{{R: 244, G: 247, B: 251, A: 255}, {R: 27, G: 33, B: 43, A: 255}} {
		draw.Draw(preview, image.Rect(0, row*rowHeight, preview.Bounds().Dx(), (row+1)*rowHeight), image.NewUniform(background), image.Point{}, draw.Src)
		for i, group := range groups {
			source, err := os.Open(filepath.Join(dir, group.ID+"-master.png"))
			if err != nil {
				return err
			}
			img, err := png.Decode(source)
			source.Close()
			if err != nil {
				return err
			}
			x, y := i*cell, row*rowHeight
			for j, size := range []int{128, 32, 16} {
				top, left := y+12, x+(cell-size)/2
				if j > 0 {
					top, left = y+156+(32-size)/2, x+53+(j-1)*55
				}
				draw.CatmullRom.Scale(preview, image.Rect(left, top, left+size, top+size), img, img.Bounds(), draw.Over, nil)
			}
			ink := color.NRGBA{R: 44, G: 62, B: 85, A: 255}
			if row == 1 {
				ink = color.NRGBA{R: 225, G: 232, B: 241, A: 255}
			}
			label := strings.ToUpper(group.ID)
			text := font.Drawer{Dst: preview, Src: image.NewUniform(ink), Face: basicfont.Face7x13,
				Dot: fixed.P(x+(cell-len(label)*7)/2, y+216)}
			text.DrawString(label)
		}
	}
	f, err := os.Create(filepath.Join(dir, "preview.png"))
	if err != nil {
		return err
	}
	err = png.Encode(f, preview)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func generateAssets(dir string) error {
	source, err := os.Open(filepath.Join(dir, "app-icon-master.png"))
	if err != nil {
		return fmt.Errorf("open generated app icon master: %w", err)
	}
	master, err := png.Decode(source)
	source.Close()
	if err != nil {
		return fmt.Errorf("decode app icon master: %w", err)
	}
	bounds := master.Bounds()
	if bounds.Dx() != bounds.Dy() || bounds.Dx() < 256 {
		return fmt.Errorf("app icon master must be square and at least 256 pixels")
	}
	for _, asset := range []struct {
		name string
		size int
	}{{"icon.png", 256}, {"Logo.png", 150}, {"Logo44.png", 44}} {
		img := image.NewNRGBA(image.Rect(0, 0, asset.size, asset.size))
		draw.CatmullRom.Scale(img, img.Bounds(), master, bounds, draw.Src, nil)
		f, err := os.Create(filepath.Join(dir, asset.name))
		if err != nil {
			return err
		}
		if err = png.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	}
	return nil
}
