// SPDX-License-Identifier: Apache-2.0
package install

import (
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

	archiveutil "github.com/kungfu-systems/taolu/internal/archive"
	"github.com/kungfu-systems/taolu/internal/compiler"
)

type State struct {
	Schema     string `json:"schema"`
	Product    string `json:"product"`
	Current    string `json:"current"`
	Previous   string `json:"previous,omitempty"`
	BundleRoot string `json:"bundleRoot"`
}
type Receipt struct {
	Schema         string `json:"schema"`
	Operation      string `json:"operation"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Platform       string `json:"platform,omitempty"`
	BundleRoot     string `json:"bundleRoot"`
	ArtifactSHA256 string `json:"artifactSha256,omitempty"`
	InstalledPath  string `json:"installedPath"`
	RecordedAt     string `json:"recordedAt"`
}

func PlatformID() string {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	return runtime.GOOS + "-" + arch
}

func Install(bundle compiler.Bundle, productID, platformID, root string) (Receipt, error) {
	if err := compiler.VerifyBundle(bundle); err != nil {
		return Receipt{}, err
	}
	if platformID == "" || platformID == "auto" {
		platformID = PlatformID()
	}
	product, platform, err := selectTarget(bundle, productID, platformID)
	if err != nil {
		return Receipt{}, err
	}
	productRoot := filepath.Join(root, "products", product.ID)
	if err := ensureOwnership(productRoot, product.ID); err != nil {
		return Receipt{}, err
	}
	cache := filepath.Join(root, "cache", platform.SHA256)
	if err := fetch(platform.URL, cache, platform.SHA256, platform.Size); err != nil {
		return Receipt{}, err
	}
	version := strings.TrimPrefix(product.ReleaseTag, "v")
	versions := filepath.Join(productRoot, "versions")
	if entries, readErr := os.ReadDir(versions); readErr == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".taolu-stage-") {
				return Receipt{}, errors.New("interrupted activation staging directory requires operator cleanup")
			}
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return Receipt{}, readErr
	}
	destination := filepath.Join(versions, version)
	if _, err := os.Stat(destination); err == nil {
		return Receipt{}, fmt.Errorf("version %s is already installed", version)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Receipt{}, err
	}
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return Receipt{}, err
	}
	staging, err := os.MkdirTemp(versions, ".taolu-stage-")
	if err != nil {
		return Receipt{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()
	if platform.Archive == "file" {
		if err := archiveutil.ExtractFile(cache, staging, platform.Entrypoint); err != nil {
			return Receipt{}, err
		}
	} else if err := archiveutil.Extract(cache, platform.Archive, staging, platform.StripComponents); err != nil {
		return Receipt{}, err
	}
	entry := filepath.Join(staging, filepath.FromSlash(platform.Entrypoint))
	info, err := os.Lstat(entry)
	if err != nil || !info.Mode().IsRegular() {
		return Receipt{}, errors.New("declared entrypoint is not a regular extracted file")
	}
	if err := os.Rename(staging, destination); err != nil {
		return Receipt{}, err
	}
	committed = true
	state, _ := readState(productRoot)
	newState := State{Schema: "taolu.install-state/v1", Product: product.ID, Current: version, Previous: state.Current, BundleRoot: bundle.BundleRoot}
	if err := writeAtomic(filepath.Join(productRoot, "state.json"), newState); err != nil {
		_ = os.RemoveAll(destination)
		return Receipt{}, err
	}
	receipt := Receipt{Schema: "taolu.install-receipt/v1", Operation: "install", Product: product.ID, Version: version, Platform: platformID, BundleRoot: bundle.BundleRoot, ArtifactSHA256: platform.SHA256, InstalledPath: destination, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeReceipt(productRoot, receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func Rollback(productID, root string) (Receipt, error) {
	productRoot := filepath.Join(root, "products", productID)
	if err := ensureExistingOwnership(productRoot, productID); err != nil {
		return Receipt{}, err
	}
	state, err := readState(productRoot)
	if err != nil {
		return Receipt{}, err
	}
	if state.Previous == "" {
		return Receipt{}, errors.New("no previous version is available")
	}
	if info, err := os.Stat(filepath.Join(productRoot, "versions", state.Previous)); err != nil || !info.IsDir() {
		return Receipt{}, errors.New("previous version is unavailable")
	}
	state.Current, state.Previous = state.Previous, state.Current
	if err := writeAtomic(filepath.Join(productRoot, "state.json"), state); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Schema: "taolu.install-receipt/v1", Operation: "rollback", Product: productID, Version: state.Current, BundleRoot: state.BundleRoot, InstalledPath: filepath.Join(productRoot, "versions", state.Current), RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeReceipt(productRoot, receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func Status(productID, root string) (State, error) {
	productRoot := filepath.Join(root, "products", productID)
	if err := ensureExistingOwnership(productRoot, productID); err != nil {
		return State{}, err
	}
	return readState(productRoot)
}

func selectTarget(bundle compiler.Bundle, productID, platformID string) (compiler.BundleProduct, compiler.BundlePlatform, error) {
	for _, p := range bundle.Products {
		if p.ID == productID {
			for _, platform := range p.Platforms {
				if platform.ID == platformID {
					return p, platform, nil
				}
			}
			return p, compiler.BundlePlatform{}, fmt.Errorf("unsupported platform %s", platformID)
		}
	}
	return compiler.BundleProduct{}, compiler.BundlePlatform{}, fmt.Errorf("unknown product %s", productID)
}

func ensureOwnership(root, product string) error {
	ownerPath := filepath.Join(root, "owner.json")
	if b, err := os.ReadFile(ownerPath); err == nil {
		var owner struct{ Schema, Product string }
		if json.Unmarshal(b, &owner) != nil || owner.Schema != "taolu.ownership/v1" || owner.Product != product {
			return errors.New("installation root is owned by another product or tool")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) > 0 {
		return errors.New("refusing to claim a non-empty foreign installation root")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAtomic(ownerPath, map[string]string{"schema": "taolu.ownership/v1", "product": product})
}
func ensureExistingOwnership(root, product string) error {
	if _, err := os.Stat(filepath.Join(root, "owner.json")); err != nil {
		return errors.New("Taolu ownership record is missing")
	}
	return ensureOwnership(root, product)
}

func fetch(rawURL, destination, digest string, expectedSize int64) error {
	if b, err := os.ReadFile(destination); err == nil {
		if int64(len(b)) == expectedSize && sha(b) == digest {
			return nil
		}
		return errors.New("cached artifact digest mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	part := destination + ".part"
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme == "file" {
		path := u.Path
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		src, err := os.Open(filepath.FromSlash(path))
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(part)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, src)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else if u.Scheme == "https" {
		var offset int64
		if info, err := os.Stat(part); err == nil {
			offset = info.Size()
		}
		req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		client := &http.Client{Timeout: 30 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		flags := os.O_CREATE | os.O_WRONLY
		if resp.StatusCode == http.StatusPartialContent {
			flags |= os.O_APPEND
		} else if resp.StatusCode == http.StatusOK {
			flags |= os.O_TRUNC
		} else {
			return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
		}
		out, err := os.OpenFile(part, flags, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, resp.Body)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else {
		return errors.New("only https and file URLs are supported")
	}
	b, err := os.ReadFile(part)
	if err != nil {
		return err
	}
	if int64(len(b)) != expectedSize || sha(b) != digest {
		_ = os.Remove(part)
		return errors.New("downloaded artifact digest mismatch")
	}
	return os.Rename(part, destination)
}
func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func readState(root string) (State, error) {
	var state State
	b, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return state, err
	}
	if state.Schema != "taolu.install-state/v1" {
		return state, errors.New("unsupported install state")
	}
	return state, nil
}
func writeAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func writeReceipt(root string, receipt Receipt) error {
	b, _ := json.Marshal(receipt)
	s := sha256.Sum256(b)
	name := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), hex.EncodeToString(s[:8]))
	return writeAtomic(filepath.Join(root, "receipts", name), receipt)
}
