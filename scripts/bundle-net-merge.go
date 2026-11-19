//go:build ignore

// Run with: go run scripts/bundle-net-merge.go [goos] [goarch] [tag]
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"netpilot/internal/netmerge/releases"
)

func main() {
	goos, arch, tag := runtime.GOOS, runtime.GOARCH, ""
	if len(os.Args) > 1 {
		goos = os.Args[1]
	}
	if len(os.Args) > 2 {
		arch = os.Args[2]
	}
	if len(os.Args) > 3 {
		tag = os.Args[3]
	}
	dir := "internal/netmerge/embedded/bin"
	versionFile := filepath.Join(dir, "release.tag")
	ctx := releases.WithDownloadToken(context.Background(), os.Getenv("GH_TOKEN"))
	resolved, files, err := releases.Download(ctx, tag, goos, arch)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o755); err != nil {
			log.Fatal(err)
		}
	}
	if goos == "darwin" {
		arch = "universal"
	}
	if err := os.WriteFile(versionFile, []byte(resolved+" "+goos+"/"+arch+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Bundled official net-merge", resolved, "for", goos+"/"+arch)
}
