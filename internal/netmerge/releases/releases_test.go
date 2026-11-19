package releases

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestUnpackOnlyReturnsKnownRegularFiles(t *testing.T) {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	w := tar.NewWriter(z)
	for _, f := range []struct{ name, content string }{{"./net-merge", "engine"}, {"../unrelated", "ignore"}} {
		if err := w.WriteHeader(&tar.Header{Name: f.name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(f.content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := unpack(b.Bytes(), "linux")
	if err != nil || len(files) != 1 || string(files["net-merge"]) != "engine" {
		t.Fatalf("unpack = %v, %v", files, err)
	}
}

func TestReleaseRequiresPlatformAssetAndDigest(t *testing.T) {
	r := Release{Tag: "v0.3.7", Assets: []Asset{{Name: "net-merge-linux-aarch64.tar.gz", Digest: "sha256:"}}}
	if _, ok := r.asset("linux", "arm64"); ok {
		t.Fatal("missing digest accepted")
	}
	r.Assets[0].Digest += "b0dafa703b5dceec7f8daf935cbfad849f085701ffcc9dfa2ae67a60744b5796"
	if _, ok := r.asset("linux", "arm64"); !ok {
		t.Fatal("valid asset was rejected")
	}
	if _, ok := r.asset("linux", "amd64"); ok {
		t.Fatal("wrong architecture was accepted")
	}
	r.Prerelease = true
	if _, ok := r.asset("linux", "arm64"); ok {
		t.Fatal("prerelease was accepted")
	}
}
