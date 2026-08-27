// SPDX-License-Identifier: Apache-2.0
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Extract(source, kind, destination string, strip int) error {
	if strip < 0 {
		return errors.New("stripComponents must be non-negative")
	}
	switch kind {
	case "zip":
		return extractZip(source, destination, strip)
	case "tar.gz":
		return extractTarGz(source, destination, strip)
	case "file":
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return err
		}
		return copyRegular(source, filepath.Join(destination, filepath.Base(source)), 0o755)
	default:
		return fmt.Errorf("unsupported archive %q", kind)
	}
}

// ExtractFile gives raw release assets their declared entrypoint name instead
// of leaking the content-addressed cache filename into the install tree.
func ExtractFile(source, destination, entrypoint string) error {
	rel, ok := safePath(entrypoint, 0)
	if !ok || rel == "" {
		return errors.New("unsafe file entrypoint")
	}
	target := filepath.Join(destination, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return copyRegular(source, target, 0o755)
}

func safePath(name string, strip int) (string, bool) {
	name = filepath.ToSlash(name)
	if strings.HasPrefix(name, "/") {
		return "", false
	}
	parts := strings.Split(strings.TrimSuffix(name, "/"), "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", false
		}
	}
	if len(parts) <= strip {
		return "", true
	}
	return filepath.Join(parts[strip:]...), true
}

func extractZip(source, destination string, strip int) error {
	zr, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		rel, ok := safePath(f.Name, strip)
		if !ok {
			return fmt.Errorf("unsafe archive entry %q", f.Name)
		}
		if rel == "" {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 || !f.Mode().IsRegular() && !f.FileInfo().IsDir() {
			return fmt.Errorf("unsupported archive entry %q", f.Name)
		}
		target := filepath.Join(destination, rel)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		err = writeReader(target, r, f.Mode().Perm())
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(source, destination string, strip int) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		rel, ok := safePath(h.Name, strip)
		if !ok {
			return fmt.Errorf("unsafe archive entry %q", h.Name)
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(destination, rel)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeReader(target, tr, os.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", h.Name)
		}
	}
	return nil
}

func writeReader(path string, r io.Reader, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
func copyRegular(source, target string, mode os.FileMode) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeReader(target, f, mode)
}
