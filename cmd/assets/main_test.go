package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeMaster(t *testing.T, dir string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := h / 4; y < 3*h/4; y++ {
		for x := w / 4; x < 3*w/4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 12, G: 95, B: 201, A: 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "app-icon-master.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAssetsComeFromMasterAndKeepTransparency(t *testing.T) {
	dir := t.TempDir()
	writeMaster(t, dir, 512, 512)
	if err := generateAssets(dir); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int{"icon.png": 256, "Logo.png": 150, "Logo44.png": 44} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds() != image.Rect(0, 0, size, size) {
			t.Fatalf("%s: wrong dimensions %v", name, img.Bounds())
		}
		if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
			t.Fatalf("%s: lost alpha transparency", name)
		}
		c := color.NRGBAModel.Convert(img.At(size/2, size/2)).(color.NRGBA)
		if c != (color.NRGBA{R: 12, G: 95, B: 201, A: 255}) {
			t.Fatalf("%s: master replaced by placeholder artwork: %v", name, c)
		}
	}
}

func TestInvalidMasterDoesNotGeneratePlaceholder(t *testing.T) {
	for _, shape := range [][2]int{{0, 0}, {128, 128}, {512, 256}} {
		dir := t.TempDir()
		if shape[0] != 0 {
			writeMaster(t, dir, shape[0], shape[1])
		}
		if err := generateAssets(dir); err == nil {
			t.Fatalf("accepted master %v", shape)
		}
		if _, err := os.Stat(filepath.Join(dir, "icon.png")); !os.IsNotExist(err) {
			t.Fatal("invalid master generated or changed an icon")
		}
	}
}

func TestProjectMasterIsTransparentPNG(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "assets", "app-icon-master.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	if bounds.Dx() < 256 || bounds.Dx() != bounds.Dy() {
		t.Fatal("application master is not a sufficiently large square")
	}
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("master has an opaque background")
	}
	if _, _, _, alpha := img.At(bounds.Dx()/2, bounds.Dy()/2).RGBA(); alpha == 0 {
		t.Fatal("master has no central artwork")
	}
}
