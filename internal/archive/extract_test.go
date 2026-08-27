// SPDX-License-Identifier: Apache-2.0
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFileUsesDeclaredEntrypoint(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "content-addressed-cache")
	if err := os.WriteFile(source, []byte("taolu"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "out")
	if err := ExtractFile(source, destination, "bin/taolu"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(destination, "bin", "taolu"))
	if err != nil || string(b) != "taolu" {
		t.Fatalf("declared raw-file entrypoint was not produced: %v", err)
	}
	if err := ExtractFile(source, destination, "../escape"); err == nil {
		t.Fatal("expected unsafe entrypoint rejection")
	}
}

func TestZipRejectsTraversal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.zip")
	f, _ := os.Create(p)
	w := zip.NewWriter(f)
	e, _ := w.Create("../escape")
	_, _ = e.Write([]byte("x"))
	_ = w.Close()
	_ = f.Close()
	if err := Extract(p, "zip", filepath.Join(t.TempDir(), "out"), 0); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
func TestTarRejectsSymlink(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.tar.gz")
	f, _ := os.Create(p)
	gz := gzip.NewWriter(f)
	w := tar.NewWriter(gz)
	_ = w.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/tmp/escape"})
	_ = w.Close()
	_ = gz.Close()
	_ = f.Close()
	if err := Extract(p, "tar.gz", filepath.Join(t.TempDir(), "out"), 0); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
