package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		want            bool
	}{
		{"0.3.3", "v0.3.4", true},
		{"v0.3.3", "v0.3.4", true},
		{"0.3.3", "v0.3.3", false},
		{"0.3.4", "v0.3.3", false},
		{"0.3.10", "v0.3.9", false}, // numbers, not text
		{"0.9.9", "v0.10.0", true},
		{"0.3", "v0.3.0", false}, // missing numbers count as zero
		{"0.3.3-dev+abc", "v0.3.4", true},
		{"0.3.3", "v0.3.4-rc1", true}, // GitHub leaves prereleases out, but be able to read one
		{"dev", "v0.3.4", false},      // a build from source has nothing to compare
		{"0.3.3", "dev", false},
		// Prerelease ordering.
		{"0.4.0-beta.1", "v0.4.0", true},
		{"0.4.0", "v0.4.0-beta.1", false},
		{"0.4.0-beta.1", "v0.4.0-beta.2", true},
		{"0.4.0-beta.2", "v0.4.0-beta.1", false},
		{"0.4.0-beta.9", "v0.4.0-beta.10", true},
		{"0.4.0-alpha", "v0.4.0-beta", true},
		{"0.4.0-beta", "v0.4.0-beta.1", true}, // more identifiers win when the prefix matches
		{"0.4.0-rc.1", "v0.4.0", true},
	} {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestIsVersionAndPrerelease(t *testing.T) {
	for v, want := range map[string]bool{
		"0.3.3":          true,
		"v1.2":           true,
		"0.3.3-dev+menu": true,
		"dev":            false,
		"":               false,
	} {
		if got := IsVersion(v); got != want {
			t.Errorf("IsVersion(%q) = %v, want %v", v, got, want)
		}
	}
	for v, want := range map[string]bool{
		"0.4.0-beta.1": true,
		"0.4.0-rc.2":   true,
		"0.4.0":        false,
		"dev":          false,
		"":             false,
	} {
		if got := IsPrerelease(v); got != want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestChannelForVersion(t *testing.T) {
	for v, want := range map[string]Channel{
		"0.4.0":        ChannelStable,
		"0.4.0-beta.1": ChannelBeta,
		"0.4.0-rc.1":   ChannelBeta,
	} {
		if got := ChannelForVersion(v); got != want {
			t.Errorf("ChannelForVersion(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestNormalizeChannel(t *testing.T) {
	for in, want := range map[string]Channel{
		"stable":     ChannelStable,
		" Stable ":   ChannelStable,
		"beta":       ChannelBeta,
		"preview":    ChannelBeta,
		"prerelease": ChannelBeta,
	} {
		got, ok := NormalizeChannel(in)
		if !ok || got != want {
			t.Errorf("NormalizeChannel(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	if _, ok := NormalizeChannel("nightly"); ok {
		t.Fatal("unknown channel must be rejected")
	}
}

func TestLatestReleaseFiltersChannels(t *testing.T) {
	body := `[
		{"tag_name":"v0.5.0-beta.2","prerelease":true,"assets":[]},
		{"tag_name":"v0.4.0","prerelease":false,"assets":[]},
		{"tag_name":"v0.4.1-beta.1","prerelease":true,"assets":[]},
		{"tag_name":"v0.3.9","prerelease":false,"assets":[]},
		{"tag_name":"v9.9.9","draft":true,"prerelease":false,"assets":[]}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases" {
			t.Errorf("asked for %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub wants a User-Agent")
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
		}
		io.WriteString(w, body)
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	stable, err := LatestRelease(context.Background(), ChannelStable)
	if err != nil {
		t.Fatalf("LatestRelease(stable): %v", err)
	}
	if stable.Tag != "v0.4.0" {
		t.Fatalf("stable = %q, want v0.4.0", stable.Tag)
	}

	beta, err := LatestRelease(context.Background(), ChannelBeta)
	if err != nil {
		t.Fatalf("LatestRelease(beta): %v", err)
	}
	if beta.Tag != "v0.5.0-beta.2" {
		t.Fatalf("beta = %q, want v0.5.0-beta.2", beta.Tag)
	}
}

func TestListReleasesFollowsPagination(t *testing.T) {
	pages := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		pages[page]++
		if page == "1" {
			var b strings.Builder
			b.WriteString("[")
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"tag_name":"v0.1.%d","prerelease":false}`, i)
			}
			b.WriteString("]")
			io.WriteString(w, b.String())
			return
		}
		io.WriteString(w, `[{"tag_name":"v0.5.0","prerelease":false}]`)
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	rel, err := LatestRelease(context.Background(), ChannelStable)
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if rel.Tag != "v0.5.0" {
		t.Fatalf("latest = %q, want the stable release on page 2", rel.Tag)
	}
	if pages["1"] != 1 || pages["2"] != 1 {
		t.Fatalf("pages fetched = %#v, want one request for pages 1 and 2", pages)
	}
}

func TestStableChannelSeesMigratedBetaAsStable(t *testing.T) {
	// A migrated v0.4.0 tag may retain GitHub's prerelease marker but must still
	// be eligible on the stable channel once the tag is the v0.4.0 line.
	body := `[{"tag_name":"v0.4.0","prerelease":false}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	rel, err := LatestRelease(context.Background(), ChannelStable)
	if err != nil {
		t.Fatalf("LatestRelease(stable): %v", err)
	}
	if rel.Tag != "v0.4.0" || rel.IsPrerelease() {
		t.Fatalf("stable unexpectedly selected %q prerelease=%v", rel.Tag, rel.IsPrerelease())
	}
}

func TestLatestReleaseUsesSemverNotPublicationOrder(t *testing.T) {
	body := `[
		{"tag_name":"v0.3.12","prerelease":false},
		{"tag_name":"v0.4.0-beta.1","prerelease":true},
		{"tag_name":"v0.3.9","prerelease":false}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	beta, err := LatestRelease(context.Background(), ChannelBeta)
	if err != nil {
		t.Fatalf("LatestRelease(beta): %v", err)
	}
	if beta.Tag != "v0.4.0-beta.1" {
		t.Fatalf("beta = %q, want v0.4.0-beta.1", beta.Tag)
	}
}

func TestLatestReadsTheTagAndIdentifiesItself(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases" {
			t.Errorf("asked for %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("GitHub wants a User-Agent")
		}
		io.WriteString(w, `[{"tag_name":"v0.3.4","prerelease":false}]`)
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	tag, err := Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if tag != "v0.3.4" {
		t.Fatalf("Latest = %q, want v0.3.4", tag)
	}
}

func TestLatestHandlesABigReleaseBody(t *testing.T) {
	// A release carries its notes: the reply can be far bigger than the tag we want.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"tag_name":"v0.3.4","prerelease":false,"body":"`)
		io.WriteString(w, strings.Repeat("x", 200<<10))
		io.WriteString(w, `"}]`)
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	tag, err := Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest with long release notes: %v", err)
	}
	if tag != "v0.3.4" {
		t.Fatalf("Latest = %q, want v0.3.4", tag)
	}
}

func TestLatestFailsLoudly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	if _, err := Latest(context.Background()); err == nil {
		t.Fatal("a 403 must be an error, not an empty tag")
	}
}

func TestParseChecksums(t *testing.T) {
	sum := strings.Repeat("a", 64)
	in := sum + "  netpilot-0.4.0-linux-amd64.AppImage\n" +
		strings.Repeat("b", 64) + " *netpilot-0.4.0-setup.exe\n" +
		"# comment\n" +
		"not-a-checksum  other.txt\n"
	sums, err := ParseChecksums(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if sums["netpilot-0.4.0-linux-amd64.AppImage"] != sum {
		t.Fatalf("missing AppImage checksum: %#v", sums)
	}
	if sums["netpilot-0.4.0-setup.exe"] != strings.Repeat("b", 64) {
		t.Fatalf("missing star-prefixed checksum: %#v", sums)
	}
	if len(sums) != 2 {
		t.Fatalf("parsed %d checksums, want 2: %#v", len(sums), sums)
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset")
	if err := os.WriteFile(path, []byte("netpilot"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte("netpilot"))
	if err := VerifyFileSHA256(path, hex.EncodeToString(h[:])); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := VerifyFileSHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("bad checksum accepted")
	}
}

func TestDownloadAndVerifyAsset(t *testing.T) {
	payload := []byte("new netpilot build")
	h := sha256.Sum256(payload)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.Write(payload)
		case "/checksums.txt":
			io.WriteString(w, hex.EncodeToString(h[:])+"  netpilot-0.4.0.AppImage\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rel := &Release{
		Tag: "v0.4.0",
		Assets: []Asset{
			{Name: "netpilot-0.4.0.AppImage", URL: srv.URL + "/asset", Size: int64(len(payload))},
			{Name: "checksums.txt", URL: srv.URL + "/checksums.txt"},
		},
	}
	dest := filepath.Join(t.TempDir(), "asset")
	var lastDone int64
	if err := Download(context.Background(), rel.Assets[0], dest, func(done, total int64) {
		lastDone = done
	}); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if lastDone != int64(len(payload)) {
		t.Fatalf("progress = %d, want %d", lastDone, len(payload))
	}
	verified, err := VerifyAsset(context.Background(), rel, rel.Assets[0], dest)
	if err != nil {
		t.Fatalf("VerifyAsset: %v", err)
	}
	if !verified {
		t.Fatal("checksum file should have been used")
	}
}

func TestDownloadRejectsTruncatedAssets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "short")
	}))
	defer srv.Close()
	asset := Asset{Name: "asset", URL: srv.URL, Size: 100}
	err := Download(context.Background(), asset, filepath.Join(t.TempDir(), "asset"), nil)
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("Download error = %v, want truncation", err)
	}
}

func TestVerifyAssetFallsBackToGitHubDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "netpilot-package")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	rel := &Release{Tag: "v0.4.0", Assets: []Asset{{
		Name:   "netpilot-0.4.0.AppImage",
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
	}}}
	verified, err := VerifyAsset(context.Background(), rel, rel.Assets[0], path)
	if err != nil {
		t.Fatalf("VerifyAsset: %v", err)
	}
	if !verified {
		t.Fatal("an asset digest should have been used")
	}

	// No checksum file and no digest: report that verification was impossible
	// instead of pretending the file is bad.
	rel.Assets[0].Digest = ""
	verified, err = VerifyAsset(context.Background(), rel, rel.Assets[0], path)
	if err != nil || verified {
		t.Fatalf("unverifiable asset = (%v, %v), want (false, nil)", verified, err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := LoadSettings(dir, "0.4.0-beta.1"); got.Channel != ChannelBeta || !got.AutoCheck {
		t.Fatalf("defaults = %+v, want beta + auto-check", got)
	}
	want := Settings{Channel: ChannelStable, AutoCheck: false, LastNotified: "v0.4.0"}
	if err := SaveSettings(dir, want); err != nil {
		t.Fatal(err)
	}
	got := LoadSettings(dir, "0.4.0-beta.1")
	if got != want {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
}

func TestReleaseChannelMarkers(t *testing.T) {
	for _, c := range []struct {
		rel  Release
		want bool
	}{
		{Release{Tag: "v0.4.0"}, false},
		{Release{Tag: "v0.4.0", Prerelease: true}, true},
		{Release{Tag: "v0.4.0-beta.1"}, true},
	} {
		if got := c.rel.IsPrerelease(); got != c.want {
			t.Errorf("IsPrerelease(%+v) = %v, want %v", c.rel, got, c.want)
		}
	}
}

func TestFindAssetUsesPriorityOrder(t *testing.T) {
	rel := &Release{Tag: "v0.4.0", Assets: []Asset{
		{Name: "netpilot-0.4.0-windows-amd64-portable.zip"},
		{Name: "netpilot-0.4.0-installer.exe"},
	}}
	got, err := findAsset(rel, "-installer.exe", "-windows-amd64-portable.zip")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "netpilot-0.4.0-installer.exe" {
		t.Fatalf("findAsset = %q", got.Name)
	}
	if _, err := findAsset(rel, "-linux-amd64.AppImage"); err == nil {
		t.Fatal("missing platform should be an error")
	}
}

func TestPageURLFallsBackToReleases(t *testing.T) {
	if got := (&Release{HTMLURL: "https://example.test/release"}).PageURL(); got != "https://example.test/release" {
		t.Fatalf("PageURL = %q", got)
	}
	if got := (&Release{}).PageURL(); got != ReleasesPageURL {
		t.Fatalf("fallback PageURL = %q", got)
	}
}

func TestDownloadUsesUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		io.WriteString(w, "x")
	}))
	defer srv.Close()
	asset := Asset{Name: "asset", URL: srv.URL, Size: 1}
	if err := Download(context.Background(), asset, filepath.Join(t.TempDir(), "asset"), nil); err != nil {
		t.Fatal(err)
	}
	if ua == "" {
		t.Fatal("download request must identify itself")
	}
}

func TestDownloadHonoursContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			io.WriteString(w, "x")
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	asset := Asset{Name: "asset", URL: srv.URL, Size: 1}
	if err := Download(ctx, asset, filepath.Join(t.TempDir(), "asset"), nil); err == nil {
		t.Fatal("cancelled download returned nil")
	}
}
