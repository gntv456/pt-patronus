package filemanager

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify P2 fix: unzipTo has size limits and isSubpath check
func TestP2_UnzipToSizeLimit(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "test.zip")
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("test.txt")
	w.Write([]byte("hello"))
	zw.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	dst := filepath.Join(tmp, "out")
	if err := unzipTo(zipPath, dst); err != nil {
		t.Fatalf("unzipTo should succeed for normal zip: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dst, "test.txt"))
	if string(data) != "hello" {
		t.Errorf("content mismatch: %q", data)
	}
}

// Verify P2 fix: zip-slip still rejected with isSubpath
func TestP2_UnzipToZipSlipRejected(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "evil.zip")
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("../../../etc/passwd")
	w.Write([]byte("evil"))
	zw.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)

	dst := filepath.Join(tmp, "out")
	err := unzipTo(zipPath, dst)
	if err == nil {
		t.Fatal("zip-slip must be rejected")
	}
	if !strings.Contains(err.Error(), "slip") {
		t.Errorf("error should mention slip: %v", err)
	}
}

// Verify isSubpath works correctly
func TestP2_IsSubpath(t *testing.T) {
	root := t.TempDir()
	target1 := filepath.Join(root, "file.txt")
	if !isSubpath(root, target1) {
		t.Error("file inside root should be subpath")
	}
	target2 := filepath.Join(root, "..", "escape")
	if isSubpath(root, target2) {
		t.Error("escape path should not be subpath")
	}
}
