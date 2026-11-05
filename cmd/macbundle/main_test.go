package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDocumentIconsContainHighResolutionPNGs(t *testing.T) {
	master := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	master.SetNRGBA(512, 512, color.NRGBA{R: 255, A: 255})
	data, err := encodeICNS(master)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
		t.Fatal("invalid ICNS header")
	}
	want := map[string]int{"icp4": 16, "icp5": 32, "icp6": 64, "ic07": 128, "ic08": 256, "ic09": 512, "ic10": 1024}
	for offset := 8; offset < len(data); {
		if offset+8 > len(data) {
			t.Fatal("truncated ICNS")
		}
		tag := string(data[offset : offset+4])
		length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if length < 8 || offset+length > len(data) {
			t.Fatal("invalid ICNS length")
		}
		img, err := png.Decode(bytes.NewReader(data[offset+8 : offset+length]))
		if err != nil || img.Bounds().Dx() != want[tag] || img.Bounds().Dy() != want[tag] {
			t.Fatalf("invalid %s image: %v", tag, err)
		}
		delete(want, tag)
		offset += length
	}
	if len(want) != 0 {
		t.Fatal("missing icon sizes", want)
	}
}
