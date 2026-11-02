package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGitHubReleaseUpdateIntegration(t *testing.T) {
	if os.Getenv("TEXTVIEWER_UPDATE_LIVE_CHECK") != "1" {
		t.Skip("set TEXTVIEWER_UPDATE_LIVE_CHECK=1 to verify the public release")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client := NewClient()
	client.cacheDir = t.TempDir()
	release, err := client.Check(ctx, "0.0.0")
	if err != nil || release == nil {
		t.Fatalf("latest public release: %+v, %v", release, err)
	}
	if expected := os.Getenv("TEXTVIEWER_EXPECTED_RELEASE"); expected != "" && release.Version != expected {
		t.Fatalf("latest release %s, expected %s", release.Version, expected)
	}
	if newer, err := client.Check(ctx, release.Version); err != nil || newer != nil {
		t.Fatalf("current version incorrectly offered an update: %+v, %v", newer, err)
	}
	installer, err := client.Download(ctx, release, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = installer.Verify(); err != nil {
		t.Fatal(err)
	}
	t.Logf("Verified public v%s installer %s: %d bytes, SHA-256 %s; installer was not executed",
		release.Version, filepath.Base(installer.Path), installer.size, installer.checksum)
}
