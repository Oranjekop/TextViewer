package main

import "testing"

func TestEmbeddedVersionIsStable(t *testing.T) {
	if version := applicationVersion(); version == "" {
		t.Fatal("missing embedded version")
	}
}
