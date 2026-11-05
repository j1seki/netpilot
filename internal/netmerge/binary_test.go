package netmerge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBinaryResolutionAndValidation tests:
// 1. Validation of the real net-merge binary.
// 2. Resolution when net-merge is located in a different location via CustomPath.
// 3. Resolution when net-merge is located in the same folder as the app.
// 4. Proper error reporting when binary is missing or invalid.
func TestBinaryResolutionAndValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping Unix binary test on Windows")
	}

	// 1. Locate our downloaded net-merge binary for testing
	downloadedBin := "/tmp/netmerge-download/net-merge"
	if _, err := os.Stat(downloadedBin); err != nil {
		t.Skip("downloaded net-merge binary not present at " + downloadedBin)
	}

	// Test ValidateBinary directly with real binary
	ver, err := ValidateBinary(downloadedBin)
	if err != nil {
		t.Fatalf("ValidateBinary failed on valid binary: %v", err)
	}
	if !strings.Contains(ver, "net-merge") {
		t.Fatalf("expected version output to contain 'net-merge', got %q", ver)
	}
	t.Logf("Validated net-merge version: %s", ver)

	// Test ValidateBinary with a non-existent path
	_, err = ValidateBinary("/path/that/does/not/exist/at/all")
	if err == nil {
		t.Fatalf("expected error for non-existent binary, got nil")
	}

	// Test ValidateBinary with a directory
	tmpDir := t.TempDir()
	_, err = ValidateBinary(tmpDir)
	if err == nil {
		t.Fatalf("expected error when validating directory, got nil")
	}

	// 2. Test Different Location via CustomPath
	customDir := filepath.Join(tmpDir, "custom-location")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatal(err)
	}
	customBin := filepath.Join(customDir, "net-merge")

	// Copy real binary to custom location
	content, err := os.ReadFile(downloadedBin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customBin, content, 0755); err != nil {
		t.Fatal(err)
	}

	info := ResolveBinary(customBin)
	if !info.IsValid || !info.Exists {
		t.Fatalf("expected custom binary to be valid and exist, got: %+v", info)
	}
	if info.Source != "custom" {
		t.Fatalf("expected source 'custom', got %q", info.Source)
	}
	if info.Path != customBin {
		t.Fatalf("expected path %q, got %q", customBin, info.Path)
	}
	t.Logf("Different location test passed: %+v", info)

	// 3. Test Same Folder Resolution
	// Create a dummy "app directory" containing both dummy netpilot and real net-merge
	sameFolderDir := filepath.Join(tmpDir, "app-folder")
	if err := os.MkdirAll(sameFolderDir, 0755); err != nil {
		t.Fatal(err)
	}
	sameFolderBin := filepath.Join(sameFolderDir, "net-merge")
	if err := os.WriteFile(sameFolderBin, content, 0755); err != nil {
		t.Fatal(err)
	}

	// Test with current working directory or environment simulation
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.Chdir(sameFolderDir); err != nil {
		t.Fatal(err)
	}

	sameFolderInfo := ResolveBinary("")
	if !sameFolderInfo.IsValid || !sameFolderInfo.Exists {
		t.Fatalf("expected same-folder binary to be valid and exist, got: %+v", sameFolderInfo)
	}
	if sameFolderInfo.Source != "same-folder" {
		t.Fatalf("expected source 'same-folder', got %q", sameFolderInfo.Source)
	}
	t.Logf("Same folder test passed: %+v", sameFolderInfo)

	// 4. Test Controller Integration: set custom path, save and reload
	stateDir := filepath.Join(tmpDir, "state")
	ctrl := NewController(stateDir)

	// Initially with no custom path in clean state
	initialInfo := ctrl.GetBinaryInfo()
	t.Logf("Initial controller binary info: %+v", initialInfo)

	// Set custom path to our custom location
	updatedInfo, err := ctrl.SetCustomBinaryPath(customBin)
	if err != nil {
		t.Fatalf("SetCustomBinaryPath failed: %v", err)
	}
	if !updatedInfo.IsValid || updatedInfo.Source != "custom" {
		t.Fatalf("unexpected updatedInfo: %+v", updatedInfo)
	}

	// Recreate controller to ensure persistence
	ctrl2 := NewController(stateDir)
	loadedInfo := ctrl2.GetBinaryInfo()
	if loadedInfo.CustomPath != customBin || !loadedInfo.IsValid {
		t.Fatalf("reloaded controller lost custom path: %+v", loadedInfo)
	}

	// Reset custom path back to auto-detect
	resetInfo, err := ctrl2.SetCustomBinaryPath("")
	if err != nil {
		t.Fatalf("failed to reset custom path: %v", err)
	}
	if resetInfo.CustomPath != "" {
		t.Fatalf("expected empty custom path after reset, got %q", resetInfo.CustomPath)
	}
	t.Logf("Controller persistence test passed")
}
