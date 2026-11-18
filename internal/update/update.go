// Package update resolves GitHub release channels, downloads newer NetPilot builds
// and starts the platform installer. Stable releases are GitHub releases that are
// neither drafts nor prereleases; the beta channel also accepts prereleases.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases are read from.
const Repo = "j1seki/netpilot"

// ReleasesURL is the canonical "latest stable" page on GitHub.
const ReleasesURL = "https://github.com/" + Repo + "/releases/latest"

// ReleasesPageURL lists every release, including betas.
const ReleasesPageURL = "https://github.com/" + Repo + "/releases"

const (
	// maxAssetSize guards against a wrong or hostile download filling the disk.
	maxAssetSize = 512 << 20
	// maxReleaseJSON is enough for 100 releases with long notes.
	maxReleaseJSON = 16 << 20
	// checksumAsset is the file this project's release workflow publishes.
	checksumAsset = "checksums.txt"
)

var (
	client         = &http.Client{Timeout: 20 * time.Second}
	downloadClient = &http.Client{Timeout: 0} // bounded by the caller's context
	apiBase        = "https://api.github.com" // tests point this at a local server
)

// Channel selects which GitHub releases are offered.
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelBeta   Channel = "beta"
)

// NormalizeChannel maps user input to one of the supported channels. The second
// return value is false for anything else so callers can reject it instead of
// silently switching a user to a different channel.
func NormalizeChannel(s string) (Channel, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "stable", "release":
		return ChannelStable, true
	case "beta", "preview", "prerelease", "pre-release":
		return ChannelBeta, true
	default:
		return ChannelStable, false
	}
}

// Valid reports whether c is a channel this build knows how to follow.
func (c Channel) Valid() bool {
	return c == ChannelStable || c == ChannelBeta
}

// Version is a semver version without build metadata. Missing minor/patch numbers
// count as zero, which matches how tags such as v1.2 are written.
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   []string
}

// ParseVersion reads a tag such as "v0.4.0-beta.1". It requires the leading dotted
// numbers to be numeric, and validates prerelease identifiers loosely enough to
// accept real projects while still rejecting a source build named "dev".
func ParseVersion(raw string) (Version, bool) {
	var v Version
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if s == "" {
		return v, false
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return v, false
	}
	nums := [3]int{}
	for i, p := range parts {
		if p == "" {
			return v, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		nums[i] = n
	}
	if hasPre {
		if pre == "" {
			return v, false
		}
		for _, id := range strings.Split(pre, ".") {
			if id == "" {
				return v, false
			}
			for _, r := range id {
				if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
					return v, false
				}
			}
			v.Pre = append(v.Pre, id)
		}
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, true
}

// CompareVersions orders versions according to semver, including prerelease
// ordering: 0.4.0-beta.1 < 0.4.0-beta.2 < 0.4.0 < 0.4.1.
func CompareVersions(a, b Version) int {
	if c := compareInt(a.Major, b.Major); c != 0 {
		return c
	}
	if c := compareInt(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := compareInt(a.Patch, b.Patch); c != 0 {
		return c
	}
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	for i := 0; i < len(a.Pre) && i < len(b.Pre); i++ {
		if c := compareIdentifier(a.Pre[i], b.Pre[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a.Pre), len(b.Pre))
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareIdentifier(a, b string) int {
	an, aNumeric := numericIdentifier(a)
	bn, bNumeric := numericIdentifier(b)
	switch {
	case aNumeric && bNumeric:
		if len(an) != len(bn) {
			return compareInt(len(an), len(bn))
		}
		return strings.Compare(an, bn)
	case aNumeric:
		return -1 // numeric identifiers sort before alphanumeric ones
	case bNumeric:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// numericIdentifier strips leading zeroes so "01" and "1" compare equal.
func numericIdentifier(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}
	return s, true
}

// IsVersion reports whether v starts with a parseable semver version.
func IsVersion(v string) bool {
	_, ok := ParseVersion(v)
	return ok
}

// IsPrerelease reports whether a version has a semver prerelease suffix.
func IsPrerelease(v string) bool {
	parsed, ok := ParseVersion(v)
	return ok && len(parsed.Pre) > 0
}

// ChannelForVersion picks the channel a freshly installed build should follow:
// beta builds stay on beta, stable builds follow stable.
func ChannelForVersion(v string) Channel {
	if IsPrerelease(v) {
		return ChannelBeta
	}
	return ChannelStable
}

// Newer reports whether release tag latest is newer than current. A stable version
// is newer than its own prereleases, so 0.4.0-beta.1 updates to 0.4.0.
func Newer(current, latest string) bool {
	c, ok := ParseVersion(current)
	if !ok {
		return false
	}
	l, ok := ParseVersion(latest)
	if !ok {
		return false
	}
	return CompareVersions(l, c) > 0
}

// Asset is one downloadable file attached to a GitHub release.
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest,omitempty"`
}

// Release is the subset of the GitHub release API NetPilot uses.
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []Asset   `json:"assets"`
}

// Version returns the release's parsed semver tag.
func (r *Release) Version() (Version, bool) {
	if r == nil {
		return Version{}, false
	}
	return ParseVersion(r.Tag)
}

// IsPrerelease reports whether GitHub marks the release as a prerelease or the
// tag itself carries a semver prerelease suffix. Either one keeps it out of the
// stable channel even if the other marker was forgotten.
func (r *Release) IsPrerelease() bool {
	if r == nil {
		return false
	}
	if r.Prerelease {
		return true
	}
	return IsPrerelease(r.Tag)
}

// PageURL is the release page to open for this release.
func (r *Release) PageURL() string {
	if r != nil && r.HTMLURL != "" {
		return r.HTMLURL
	}
	return ReleasesPageURL
}

// ListReleases fetches the repository's published releases, newest first. It
// follows the API's pagination so a long run of betas cannot push the newest
// stable release off the first page.
func ListReleases(ctx context.Context) ([]Release, error) {
	const perPage = 100
	const maxPages = 5
	var all []Release
	for page := 1; page <= maxPages; page++ {
		u := fmt.Sprintf("%s/repos/%s/releases?per_page=%d", apiBase, Repo, perPage)
		if page > 1 {
			u += fmt.Sprintf("&page=%d", page)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "netpilot")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var batch []Release
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSON)).Decode(&batch)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github: %s", resp.Status)
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}

// LatestRelease returns the newest release this channel should offer. Stable
// ignores prereleases and drafts; beta includes prereleases but still ignores
// drafts. Channels compare semver, not publication date, so an old release
// uploaded late cannot hide a newer one.
func LatestRelease(ctx context.Context, channel Channel) (*Release, error) {
	if !channel.Valid() {
		channel = ChannelStable
	}
	releases, err := ListReleases(ctx)
	if err != nil {
		return nil, err
	}
	var best *Release
	var bestVersion Version
	for i := range releases {
		rel := &releases[i]
		if rel.Draft {
			continue
		}
		if channel == ChannelStable && rel.IsPrerelease() {
			continue
		}
		v, ok := rel.Version()
		if !ok {
			continue
		}
		if best == nil || CompareVersions(v, bestVersion) > 0 {
			best, bestVersion = rel, v
		}
	}
	if best == nil {
		return nil, fmt.Errorf("github: no %s releases found", channel)
	}
	return best, nil
}

// Latest returns the newest stable release tag. Kept for the net-merge tray,
// which has no channel setting of its own.
func Latest(ctx context.Context) (string, error) {
	rel, err := LatestRelease(ctx, ChannelStable)
	if err != nil {
		return "", err
	}
	return rel.Tag, nil
}

// findAsset returns the first attached file whose name ends with one of suffixes,
// in priority order. Release filenames include the version, so matching the stable
// platform suffix avoids guessing the tag format.
func findAsset(r *Release, suffixes ...string) (*Asset, error) {
	if r == nil {
		return nil, fmt.Errorf("no release")
	}
	for _, suffix := range suffixes {
		for i := range r.Assets {
			if strings.HasSuffix(r.Assets[i].Name, suffix) {
				a := r.Assets[i]
				return &a, nil
			}
		}
	}
	return nil, fmt.Errorf("release %s has no package for this platform", r.Tag)
}

// Checksums maps an asset filename to its lowercase SHA-256 hex digest.
type Checksums map[string]string

// ParseChecksums reads the output of sha256sum: "<hex>  <filename>".
func ParseChecksums(r io.Reader) (Checksums, error) {
	out := make(Checksums)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(sum); err != nil {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		out[filepath.Base(name)] = sum
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FetchChecksums downloads the release's checksums.txt. It returns nil, nil when
// the release has no checksum file, which keeps older releases usable.
func FetchChecksums(ctx context.Context, rel *Release) (Checksums, error) {
	if rel == nil {
		return nil, fmt.Errorf("no release")
	}
	var asset *Asset
	for _, want := range []string{checksumAsset, "SHA256SUMS", "sha256sums.txt"} {
		for i := range rel.Assets {
			if strings.EqualFold(rel.Assets[i].Name, want) {
				a := rel.Assets[i]
				asset = &a
				break
			}
		}
		if asset != nil {
			break
		}
	}
	if asset == nil {
		return nil, nil
	}
	data, err := fetchBytes(ctx, asset.URL, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", asset.Name, err)
	}
	sums, err := ParseChecksums(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	return sums, nil
}

// VerifyAsset checks the downloaded file against the release's checksums.txt or,
// when that is absent, the SHA-256 digest GitHub reports for the asset. The bool
// reports whether a checksum was available at all.
func VerifyAsset(ctx context.Context, rel *Release, asset Asset, path string) (bool, error) {
	sums, err := FetchChecksums(ctx, rel)
	if err != nil {
		return false, err
	}
	want := ""
	if sums != nil {
		want = sums[filepath.Base(asset.Name)]
		if want == "" {
			return true, fmt.Errorf("%s does not list %s", checksumAsset, asset.Name)
		}
	} else {
		want = digestSHA256(asset.Digest)
	}
	if want == "" {
		return false, nil
	}
	return true, VerifyFileSHA256(path, want)
}

func digestSHA256(digest string) string {
	algo, value, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || !strings.EqualFold(algo, "sha256") {
		return ""
	}
	value = strings.ToLower(value)
	if len(value) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

// VerifyFileSHA256 compares a file's SHA-256 digest with a hex digest.
func VerifyFileSHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, strings.TrimSpace(want)) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filepath.Base(path), got, want)
	}
	return nil
}

// Download saves an asset to dest and reports progress. The HTTP client has no
// overall timeout; a context deadline or user cancellation stops the transfer.
func Download(ctx context.Context, asset Asset, dest string, progress func(downloaded, total int64)) error {
	u, err := url.Parse(asset.URL)
	if err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	// Release assets are served over HTTPS. Plain HTTP is only accepted for
	// loopback test servers, never for a real GitHub download.
	if u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname()))) {
		return fmt.Errorf("invalid download URL %q", asset.URL)
	}
	if asset.Size > maxAssetSize {
		return fmt.Errorf("%s is too large (%d bytes)", asset.Name, asset.Size)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "netpilot")
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", asset.Name, resp.Status)
	}
	total := resp.ContentLength
	if total <= 0 {
		total = asset.Size
	}
	if total > maxAssetSize {
		return fmt.Errorf("%s is too large (%d bytes)", asset.Name, total)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 128<<10)
	var done int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			if done > maxAssetSize {
				return fmt.Errorf("%s exceeded the size limit", asset.Name)
			}
			if progress != nil {
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if asset.Size > 0 && done != asset.Size {
		return fmt.Errorf("download %s was truncated: got %d bytes, want %d", asset.Name, done, asset.Size)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func fetchBytes(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "netpilot")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
