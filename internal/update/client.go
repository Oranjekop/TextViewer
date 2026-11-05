package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Oranjekop/TextViewer/internal/releaseinfo"
)

const maxInstallerBytes = 256 << 20

var ErrRateLimited = errors.New("update checks are temporarily rate limited")

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type Release struct {
	Tag        string  `json:"tag_name"`
	Notes      string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
	Version    string  `json:"-"`
}

type Client struct {
	http       *http.Client
	latestURL  string
	repository string
	cacheDir   string
	platform   string
}

type Installer struct {
	Path     string
	checksum string
	size     int64
}

func (i *Installer) Verify() error {
	if i == nil || i.size <= 0 || i.size > maxInstallerBytes {
		return errors.New("invalid installer")
	}
	f, err := os.Open(i.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, i.size+1))
	if err != nil {
		return err
	}
	if n != i.size || hex.EncodeToString(hash.Sum(nil)) != i.checksum {
		return errors.New("downloaded installer has changed")
	}
	return nil
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 5 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Scheme != "https" || len(via) >= 10 {
					return errors.New("unsafe update redirect")
				}
				return nil
			},
		},
		latestURL:  "https://api.github.com/repos/" + releaseinfo.Repository + "/releases/latest",
		repository: "https://github.com/" + releaseinfo.Repository,
		platform:   runtime.GOOS,
	}
}

func (c *Client) Check(ctx context.Context, current string) (*Release, error) {
	if _, err := releaseinfo.ParseVersion(current); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	response, err := c.get(ctx, c.latestURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update service returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, errors.New("release metadata too large")
	}
	var release Release
	if err = json.Unmarshal(data, &release); err != nil {
		return nil, err
	}
	if release.Draft || release.Prerelease {
		return nil, nil
	}
	release.Version, err = releaseinfo.TagVersion(release.Tag)
	if err != nil {
		return nil, err
	}
	newer, err := releaseinfo.Newer(release.Version, current)
	if err != nil || !newer {
		return nil, err
	}
	if _, _, err = c.assets(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (c *Client) get(ctx context.Context, target string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "TextViewer-update")
	request.Header.Set("Accept", "application/vnd.github+json, application/octet-stream")
	return c.http.Do(request)
}

func (c *Client) assets(release *Release) (installer, checksums Asset, err error) {
	if release == nil {
		return installer, checksums, errors.New("missing release")
	}
	version, err := releaseinfo.TagVersion(release.Tag)
	if err != nil || version != release.Version {
		return installer, checksums, errors.New("inconsistent release version")
	}
	installerName := releaseinfo.InstallerNameFor(version, c.platform)
	if installerName == "" {
		return installer, checksums, errors.New("updates are unsupported on this platform")
	}
	for _, asset := range release.Assets {
		switch asset.Name {
		case installerName:
			if installer.Name != "" {
				return installer, checksums, errors.New("duplicate installer asset")
			}
			installer = asset
		case "SHA256SUMS.txt":
			if checksums.Name != "" {
				return installer, checksums, errors.New("duplicate checksum asset")
			}
			checksums = asset
		}
	}
	if installer.Size <= 0 || installer.Size > maxInstallerBytes || checksums.Size <= 0 || checksums.Size > 64<<10 {
		return installer, checksums, errors.New("release installer or checksums are missing or invalid")
	}
	for _, asset := range []Asset{installer, checksums} {
		expected := c.repository + "/releases/download/" + release.Tag + "/" + asset.Name
		parsed, parseErr := url.Parse(asset.URL)
		if parseErr != nil || parsed.User != nil || asset.URL != expected {
			return installer, checksums, errors.New("release asset is outside the expected repository")
		}
	}
	return installer, checksums, nil
}

// Download verifies the public checksum list and GitHub's digest, when present.
// It only writes a new file inside the application cache, never a user document.
func (c *Client) Download(ctx context.Context, release *Release, progress func(done, total int64)) (result *Installer, err error) {
	installer, checksums, err := c.assets(release)
	if err != nil {
		return nil, err
	}
	response, err := c.get(ctx, checksums.URL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksum download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, errors.New("invalid checksum download")
	}
	checksum, err := checksumFor(data, installer.Name)
	if err != nil {
		return nil, err
	}
	if installer.Digest != "" && !strings.EqualFold(installer.Digest, "sha256:"+checksum) {
		return nil, errors.New("GitHub asset digest disagrees with the checksum list")
	}
	cache := c.cacheDir
	if cache == "" {
		cache, err = os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		cache = filepath.Join(cache, "TextViewer", "updates")
	}
	if err = os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(cache, "TextViewer-Setup-"+release.Version+"-*"+filepath.Ext(installer.Name))
	if err != nil {
		return nil, err
	}
	path := f.Name()
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	response, err = c.get(ctx, installer.URL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("installer download returned HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	writer := &downloadWriter{dst: io.MultiWriter(f, hash), total: installer.Size, progress: progress}
	n, err := io.Copy(writer, io.LimitReader(response.Body, installer.Size+1))
	if err != nil {
		return nil, err
	}
	if n != installer.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), checksum) {
		return nil, errors.New("installer size or checksum mismatch")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = f.Sync(); err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(n, n)
	}
	return &Installer{Path: path, checksum: checksum, size: n}, nil
}

func checksumFor(data []byte, name string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	found := ""
	for scanner.Scan() {
		line := strings.Fields(scanner.Text())
		if len(line) != 2 || strings.TrimPrefix(line[1], "*") != name {
			continue
		}
		decoded, err := hex.DecodeString(line[0])
		if err != nil || len(decoded) != sha256.Size || found != "" {
			return "", errors.New("invalid or duplicate installer checksum")
		}
		found = strings.ToLower(line[0])
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("installer checksum missing")
	}
	return found, nil
}

type downloadWriter struct {
	dst      io.Writer
	total    int64
	done     int64
	last     time.Time
	progress func(done, total int64)
}

func (w *downloadWriter) Write(data []byte) (int, error) {
	n, err := w.dst.Write(data)
	w.done += int64(n)
	if w.progress != nil && time.Since(w.last) >= 200*time.Millisecond {
		w.progress(w.done, w.total)
		w.last = time.Now()
	}
	return n, err
}
