package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Oranjekop/TextViewer/internal/releaseinfo"
)

type requestLog struct {
	mu    sync.Mutex
	paths []string
}

func (r *requestLog) add(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

func (r *requestLog) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func testClient(t *testing.T, mutate func(*Release), badPayload bool) (*Client, *Release, *requestLog) {
	return testPlatformClient(t, "windows", mutate, badPayload)
}

func testPlatformClient(t *testing.T, platform string, mutate func(*Release), badPayload bool) (*Client, *Release, *requestLog) {
	t.Helper()
	name := releaseinfo.InstallerNameFor("0.0.2", platform)
	payload := []byte("MZTextViewer installer fixture")
	digest := sha256.Sum256(payload)
	sum := hex.EncodeToString(digest[:])
	requests := new(requestLog)
	var release Release
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.add(r.URL.Path)
		switch {
		case r.URL.Path == "/latest":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(release)
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS.txt"):
			fmt.Fprintf(w, "%s  %s\n", sum, name)
		case strings.HasSuffix(r.URL.Path, "/"+name):
			if badPayload {
				w.Write([]byte("corrupt installer"))
			} else {
				w.Write(payload)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient()
	client.platform = platform
	client.http.Transport = server.Client().Transport
	client.latestURL = server.URL + "/latest"
	client.repository = server.URL + "/" + releaseinfo.Repository
	client.cacheDir = t.TempDir()
	base := client.repository + "/releases/download/v0.0.2/"
	release = Release{
		Tag: "v0.0.2", Version: "0.0.2", Notes: "Test release",
		Assets: []Asset{
			{Name: name, URL: base + name, Size: int64(len(payload)), Digest: "sha256:" + sum},
			{Name: "SHA256SUMS.txt", URL: base + "SHA256SUMS.txt", Size: 200},
		},
	}
	if mutate != nil {
		mutate(&release)
	}
	return client, &release, requests
}

func TestMacUpdateDownloadsOneUniversalPackage(t *testing.T) {
	client, _, requests := testPlatformClient(t, "darwin", nil, false)
	release, err := client.Check(context.Background(), "0.0.1")
	if err != nil || release == nil {
		t.Fatalf("macOS check: %v", err)
	}
	installer, err := client.Download(context.Background(), release, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(installer.Path) != ".pkg" {
		t.Fatalf("macOS installer: %s", installer.Path)
	}
	if err := installer.Verify(); err != nil {
		t.Fatal(err)
	}
	paths := requests.snapshot()
	if len(paths) != 3 || !strings.HasSuffix(paths[2], "-macos-universal.pkg") {
		t.Fatalf("unexpected download requests: %v", paths)
	}
	client.platform = "windows"
	if _, _, err := client.assets(release); err == nil {
		t.Fatal("Windows accepted a macOS package")
	}
	client.platform = "darwin"
	release.Assets[0].Name = strings.ReplaceAll(release.Assets[0].Name, "universal", "arm64")
	if _, _, err := client.assets(release); err == nil {
		t.Fatal("accepted a package without both architectures")
	}
}

func TestCheckAndDownloadVerifiedInstaller(t *testing.T) {
	client, _, requests := testClient(t, nil, false)
	release, err := client.Check(context.Background(), "0.0.1")
	if err != nil || release == nil || release.Version != "0.0.2" {
		t.Fatalf("check: %+v %v", release, err)
	}
	if paths := requests.snapshot(); len(paths) != 1 || paths[0] != "/latest" {
		t.Fatal("checking downloaded binaries before user confirmation")
	}
	progress := false
	installer, err := client.Download(context.Background(), release, func(done, total int64) {
		progress = progress || done == total
	})
	if err != nil {
		t.Fatal(err)
	}
	if !progress || filepath.Dir(installer.Path) != client.cacheDir {
		t.Fatal("download location or progress wrong")
	}
	if err = installer.Verify(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(installer.Path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = installer.Verify(); err == nil {
		t.Fatal("accepted an installer changed after download")
	}
}

func TestNoUpdateForEqualOrOlderVersions(t *testing.T) {
	client, _, _ := testClient(t, nil, false)
	for _, version := range []string{"0.0.2", "0.0.3", "0.10.0", "1.0.0"} {
		release, err := client.Check(context.Background(), version)
		if err != nil || release != nil {
			t.Fatalf("current %s: %+v %v", version, release, err)
		}
	}
}

func TestRejectUnsafeOrIncompleteReleases(t *testing.T) {
	for name, mutate := range map[string]func(*Release){
		"other host": func(r *Release) { r.Assets[0].URL = "https://evil.invalid/setup.exe" },
		"query":      func(r *Release) { r.Assets[0].URL += "?unexpected=1" },
		"missing":    func(r *Release) { r.Assets = r.Assets[:1] },
		"oversize":   func(r *Release) { r.Assets[0].Size = maxInstallerBytes + 1 },
		"duplicate":  func(r *Release) { r.Assets = append(r.Assets, r.Assets[0]) },
		"bad tag":    func(r *Release) { r.Tag = "v0.0.3/../../other" },
	} {
		t.Run(name, func(t *testing.T) {
			client, _, _ := testClient(t, mutate, false)
			if _, err := client.Check(context.Background(), "0.0.1"); err == nil {
				t.Fatal("unsafe release was accepted")
			}
		})
	}
}

func TestDraftAndPrereleaseAreNotOffered(t *testing.T) {
	for _, draft := range []bool{true, false} {
		client, _, _ := testClient(t, func(r *Release) {
			r.Draft, r.Prerelease = draft, !draft
		}, false)
		if release, err := client.Check(context.Background(), "0.0.1"); err != nil || release != nil {
			t.Fatalf("offered draft/prerelease: %+v %v", release, err)
		}
	}
}

func TestCorruptDownloadIsRemoved(t *testing.T) {
	client, release, _ := testClient(t, nil, true)
	if installer, err := client.Download(context.Background(), release, nil); err == nil || installer != nil {
		t.Fatal("corrupt installer accepted")
	}
	entries, _ := os.ReadDir(client.cacheDir)
	if len(entries) != 0 {
		t.Fatal("failed download left an executable behind")
	}
}

func TestGitHubDigestMustAgree(t *testing.T) {
	client, release, requests := testClient(t, func(r *Release) {
		r.Assets[0].Digest = "sha256:" + strings.Repeat("a", 64)
	}, false)
	if _, err := client.Download(context.Background(), release, nil); err == nil {
		t.Fatal("conflicting GitHub digest accepted")
	}
	if paths := requests.snapshot(); len(paths) != 1 || strings.HasSuffix(paths[0], ".exe") {
		t.Fatal("downloaded executable despite a known digest mismatch")
	}
}

func TestCancelledUpdateCheck(t *testing.T) {
	client, _, _ := testClient(t, nil, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Check(ctx, "0.0.1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelledDownloadRemovesPartialInstaller(t *testing.T) {
	client, release, _ := testClient(t, nil, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	installer, err := client.Download(ctx, release, func(_, _ int64) { cancel() })
	if !errors.Is(err, context.Canceled) || installer != nil {
		t.Fatalf("cancelled download: %v, %v", installer, err)
	}
	entries, err := os.ReadDir(client.cacheDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelled download left a partial installer: %v, %v", entries, err)
	}
}

func TestRejectInsecureUpdateRedirect(t *testing.T) {
	var insecureRequests int
	var mu sync.Mutex
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		insecureRequests++
		mu.Unlock()
	}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, insecure.URL, http.StatusFound)
	}))
	defer secure.Close()
	client := NewClient()
	client.http.Transport, client.latestURL = secure.Client().Transport, secure.URL
	if _, err := client.Check(context.Background(), "0.0.1"); err == nil {
		t.Fatal("followed an insecure redirect")
	}
	mu.Lock()
	defer mu.Unlock()
	if insecureRequests != 0 {
		t.Fatal("sent a request over plain HTTP")
	}
}

func TestDownloadRejectsMissingRelease(t *testing.T) {
	if _, err := NewClient().Download(context.Background(), nil, nil); err == nil {
		t.Fatal("accepted a missing release")
	}
}

func TestChecksumsRejectMissingAndAmbiguousEntries(t *testing.T) {
	name := "setup.exe"
	sum := strings.Repeat("a", 64)
	for _, data := range []string{"", "bad  setup.exe", sum + "  other.exe", sum + "  setup.exe\n" + sum + "  setup.exe"} {
		if _, err := checksumFor([]byte(data), name); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	if got, err := checksumFor([]byte(sum+" *setup.exe\r\n"), name); err != nil || got != sum {
		t.Fatalf("valid checksum %s: %v", got, err)
	}
}

func TestNoReleaseAndRateLimit(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusTooManyRequests, http.StatusBadGateway} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		client := NewClient()
		client.http.Transport, client.latestURL = server.Client().Transport, server.URL
		release, err := client.Check(context.Background(), "0.0.1")
		server.Close()
		if status == http.StatusNotFound {
			if release != nil || err != nil {
				t.Fatal("first release not yet published should not fail")
			}
		} else if err == nil {
			t.Fatal("service failure reported as up-to-date")
		}
	}
}
