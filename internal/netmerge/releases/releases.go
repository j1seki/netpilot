// Package releases fetches platform-specific binaries from official net-merge releases.
package releases

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const repoAPI = "https://api.github.com/repos/net-merge/net-merge"
const maxAsset = 64 << 20

var tagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
var client = &http.Client{Timeout: 45 * time.Second}

func ValidTag(tag string) bool { return tagPattern.MatchString(tag) }

type Asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

func assetName(goos, arch string) string {
	switch goos + "/" + arch {
	case "windows/amd64":
		return "net-merge-portable.zip"
	case "darwin/amd64", "darwin/arm64":
		return "net-merge-macos-universal.tar.gz"
	case "linux/arm64":
		return "net-merge-linux-aarch64.tar.gz"
	case "linux/amd64":
		return "net-merge-linux-x86_64.tar.gz"
	}
	return ""
}

func (r Release) asset(goos, arch string) (Asset, bool) {
	if !ValidTag(r.Tag) || r.Draft || r.Prerelease {
		return Asset{}, false
	}
	name := assetName(goos, arch)
	for _, a := range r.Assets {
		if name != "" && a.Name == name {
			digest, ok := strings.CutPrefix(a.Digest, "sha256:")
			if decoded, err := hex.DecodeString(digest); ok && err == nil && len(decoded) == sha256.Size {
				return a, true
			}
		}
	}
	return Asset{}, false
}

func List(ctx context.Context, goos, arch string) ([]string, error) {
	var list []Release
	if err := metadata(ctx, "/releases?per_page=30", &list); err != nil {
		return nil, err
	}
	tags := []string{}
	for _, r := range list {
		if _, ok := r.asset(goos, arch); ok {
			tags = append(tags, r.Tag)
		}
	}
	return tags, nil
}

func metadata(ctx context.Context, endpoint string, dst any) error {
	b, err := get(ctx, repoAPI+endpoint, "application/vnd.github+json", 8<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func get(ctx context.Context, address, accept string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "NetPilot")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if int64(len(b)) > max {
		return nil, fmt.Errorf("GitHub response exceeds size limit")
	}
	return b, err
}

// Download verifies the archive against the SHA-256 from the official GitHub API.
// Only known basenames are returned; archive paths are never used for writes.
func Download(ctx context.Context, tag, goos, arch string) (string, map[string][]byte, error) {
	endpoint := "/releases/latest"
	if tag != "" {
		if !ValidTag(tag) {
			return "", nil, fmt.Errorf("invalid release tag")
		}
		endpoint = "/releases/tags/" + tag
	}
	var r Release
	if err := metadata(ctx, endpoint, &r); err != nil {
		return "", nil, err
	}
	a, ok := r.asset(goos, arch)
	if !ok || (tag != "" && r.Tag != tag) {
		return "", nil, fmt.Errorf("no verified net-merge release for %s/%s", goos, arch)
	}
	wantURL := "https://github.com/net-merge/net-merge/releases/download/" + r.Tag + "/" + url.PathEscape(a.Name)
	if a.URL != wantURL {
		return "", nil, fmt.Errorf("unexpected release asset URL")
	}
	b, err := get(ctx, wantURL, "application/octet-stream", maxAsset)
	if err != nil && a.ID != 0 && ctx.Err() == nil {
		b, err = get(ctx, fmt.Sprintf("%s/releases/assets/%d", repoAPI, a.ID), "application/octet-stream", maxAsset)
	}
	if err != nil {
		return "", nil, err
	}
	if a.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(b)) {
		return "", nil, fmt.Errorf("release archive checksum did not match")
	}
	files, err := unpack(b, goos)
	return r.Tag, files, err
}

func CLIName(goos string) string {
	if goos == "windows" {
		return "net-merge.exe"
	}
	return "net-merge"
}

func unpack(b []byte, goos string) (map[string][]byte, error) {
	files := map[string][]byte{}
	add := func(name string, size int64, r io.Reader) error {
		name = path.Base(name)
		if name != CLIName(goos) && !(goos == "windows" && (name == "wintun.dll" || name == "wintun-LICENSE.txt")) {
			return nil
		}
		if size <= 0 || size > maxAsset || files[name] != nil {
			return fmt.Errorf("invalid or duplicate archive entry %q", name)
		}
		data, err := io.ReadAll(io.LimitReader(r, size+1))
		if err != nil {
			return err
		}
		if int64(len(data)) != size {
			return fmt.Errorf("incorrect archive entry size for %q", name)
		}
		files[name] = data
		return nil
	}
	if goos == "windows" {
		z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, err
		}
		for _, f := range z.File {
			if !f.Mode().IsRegular() {
				continue
			}
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = add(f.Name, int64(f.UncompressedSize64), r)
			r.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		z, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer z.Close()
		r := tar.NewReader(io.LimitReader(z, 2*maxAsset))
		for {
			f, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if f.Typeflag != tar.TypeReg {
				continue
			}
			if err := add(f.Name, f.Size, r); err != nil {
				return nil, err
			}
		}
	}
	if files[CLIName(goos)] == nil || (goos == "windows" && files["wintun.dll"] == nil) {
		return nil, fmt.Errorf("release archive is missing required binaries")
	}
	return files, nil
}
