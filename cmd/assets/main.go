package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

func main() {
	dir := "assets"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := generateAssets(dir); err != nil {
		log.Fatal(err)
	}
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
