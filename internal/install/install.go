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
	Schema      string            `json:"schema"`
	Product     string            `json:"product"`
	Command     string            `json:"command"`
	Current     string            `json:"current"`
	Previous    string            `json:"previous,omitempty"`
	BundleRoot  string            `json:"bundleRoot"`
	Launcher    string            `json:"launcher,omitempty"`
	Entrypoints map[string]string `json:"entrypoints,omitempty"`
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

type VersionRecord struct {
	Schema         string `json:"schema"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Platform       string `json:"platform"`
	ArtifactSHA256 string `json:"artifactSha256"`
	Entrypoint     string `json:"entrypoint"`
}

type Options struct {
	Product  string
	Version  string
	Platform string
	Root     string
	BinDir   string
	DryRun   bool
}

func PlatformID() string {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	return runtime.GOOS + "-" + arch
}

func Install(bundle compiler.Bundle, productID, platformID, root string) (Receipt, error) {
	return InstallWithOptions(bundle, Options{Product: productID, Platform: platformID, Root: root})
}

func InstallWithOptions(bundle compiler.Bundle, options Options) (Receipt, error) {
	if err := compiler.VerifyBundle(bundle); err != nil {
		return Receipt{}, err
	}
	productID := options.Product
	platformID := options.Platform
	root := options.Root
	if platformID == "" || platformID == "auto" {
		platformID = PlatformID()
	}
	product, version, platform, err := selectTarget(bundle, productID, options.Version, platformID)
	if err != nil {
		return Receipt{}, err
	}
	if options.DryRun {
		return Receipt{Schema: "taolu.install-receipt/v1", Operation: "plan", Product: product.ID, Version: version.Version, Platform: platformID, BundleRoot: bundle.BundleRoot, ArtifactSHA256: platform.SHA256, InstalledPath: filepath.Join(root, "products", product.ID, "versions", version.Version), RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
	}
	productRoot := filepath.Join(root, "products", product.ID)
	if err := ensureOwnership(productRoot, product.ID); err != nil {
		return Receipt{}, err
	}
	lock := filepath.Join(productRoot, ".install.lock")
	if err := os.Mkdir(lock, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Receipt{}, errors.New("another Taolu installation owns the product lock")
		}
		return Receipt{}, err
	}
	defer os.Remove(lock)
	cache := filepath.Join(root, "cache", platform.SHA256)
	for _, evidence := range platform.Evidence {
		if err := fetch(evidence.URL, filepath.Join(root, "cache", evidence.SHA256), evidence.SHA256, evidence.Size); err != nil {
			return Receipt{}, fmt.Errorf("verify %s evidence: %w", evidence.Kind, err)
		}
	}
	if err := fetch(platform.URL, cache, platform.SHA256, platform.Size); err != nil {
		return Receipt{}, err
	}
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
	destination := filepath.Join(versions, version.Version)
	if info, err := os.Stat(destination); err == nil {
		if !info.IsDir() {
			return Receipt{}, fmt.Errorf("installed version path %s is not a directory", version.Version)
		}
		var record VersionRecord
		if _, err := compiler.ReadJSON(filepath.Join(destination, ".taolu-version.json"), &record); err != nil || record.Schema != "taolu.version-record/v1" || record.Product != product.ID || record.Version != version.Version || record.Platform != platformID || record.ArtifactSHA256 != platform.SHA256 || record.Entrypoint != platform.Entrypoint {
			return Receipt{}, fmt.Errorf("installed version %s does not match the exact bundle", version.Version)
		}
		state, _ := readState(productRoot)
		if state.Entrypoints == nil {
			state.Entrypoints = map[string]string{}
		}
		state.Entrypoints[version.Version] = platform.Entrypoint
		binDir := options.BinDir
		if binDir == "" {
			binDir = filepath.Join(root, "bin")
		}
		launcher, err := activateLauncher(productRoot, product.Command, filepath.Join(destination, filepath.FromSlash(platform.Entrypoint)), binDir, state.Launcher)
		if err != nil {
			return Receipt{}, err
		}
		previous := state.Current
		if previous == version.Version {
			previous = state.Previous
		}
		state = State{Schema: "taolu.install-state/v1", Product: product.ID, Command: product.Command, Current: version.Version, Previous: previous, BundleRoot: bundle.BundleRoot, Launcher: launcher, Entrypoints: state.Entrypoints}
		if err := writeAtomic(filepath.Join(productRoot, "state.json"), state); err != nil {
			return Receipt{}, err
		}
		receipt := Receipt{Schema: "taolu.install-receipt/v1", Operation: "activate", Product: product.ID, Version: version.Version, Platform: platformID, BundleRoot: bundle.BundleRoot, ArtifactSHA256: platform.SHA256, InstalledPath: destination, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if err := writeReceipt(productRoot, receipt); err != nil {
			return Receipt{}, err
		}
		return receipt, nil
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
	if platform.EntrypointSHA256 != "" {
		entryBytes, readErr := os.ReadFile(entry)
		if readErr != nil || sha(entryBytes) != platform.EntrypointSHA256 {
			return Receipt{}, errors.New("extracted entrypoint digest mismatch")
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(entry, 0o755); err != nil {
			return Receipt{}, err
		}
	}
	versionRecord := VersionRecord{Schema: "taolu.version-record/v1", Product: product.ID, Version: version.Version, Platform: platformID, ArtifactSHA256: platform.SHA256, Entrypoint: platform.Entrypoint}
	if err := writeAtomic(filepath.Join(staging, ".taolu-version.json"), versionRecord); err != nil {
		return Receipt{}, err
	}
	if err := os.Rename(staging, destination); err != nil {
		return Receipt{}, err
	}
	committed = true
	state, _ := readState(productRoot)
	if state.Entrypoints == nil {
		state.Entrypoints = map[string]string{}
	}
	state.Entrypoints[version.Version] = platform.Entrypoint
	binDir := options.BinDir
	if binDir == "" {
		binDir = filepath.Join(root, "bin")
	}
	launcher, err := activateLauncher(productRoot, product.Command, filepath.Join(destination, filepath.FromSlash(platform.Entrypoint)), binDir, state.Launcher)
	if err != nil {
		_ = os.RemoveAll(destination)
		return Receipt{}, err
	}
	newState := State{Schema: "taolu.install-state/v1", Product: product.ID, Command: product.Command, Current: version.Version, Previous: state.Current, BundleRoot: bundle.BundleRoot, Launcher: launcher, Entrypoints: state.Entrypoints}
	if err := writeAtomic(filepath.Join(productRoot, "state.json"), newState); err != nil {
		_ = os.RemoveAll(destination)
		return Receipt{}, err
	}
	receipt := Receipt{Schema: "taolu.install-receipt/v1", Operation: "install", Product: product.ID, Version: version.Version, Platform: platformID, BundleRoot: bundle.BundleRoot, ArtifactSHA256: platform.SHA256, InstalledPath: destination, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
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
	lock := filepath.Join(productRoot, ".install.lock")
	if err := os.Mkdir(lock, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Receipt{}, errors.New("another Taolu installation owns the product lock")
		}
		return Receipt{}, err
	}
	defer os.Remove(lock)
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
	entrypoint := state.Entrypoints[state.Previous]
	if entrypoint == "" || state.Launcher == "" {
		return Receipt{}, errors.New("previous version activation metadata is unavailable")
	}
	if _, err := activateLauncher(productRoot, state.Command, filepath.Join(productRoot, "versions", state.Previous, filepath.FromSlash(entrypoint)), filepath.Dir(state.Launcher), state.Launcher); err != nil {
		return Receipt{}, err
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

func selectTarget(bundle compiler.Bundle, productID, versionID, platformID string) (compiler.BundleProduct, compiler.BundleVersion, compiler.BundlePlatform, error) {
	for _, p := range bundle.Products {
		if p.ID == productID {
			if versionID == "" {
				versionID = p.DefaultVersion
			}
			for _, version := range p.Versions {
				if version.Version != versionID {
					continue
				}
				for _, platform := range version.Platforms {
					if platform.ID == platformID {
						return p, version, platform, nil
					}
				}
				return p, version, compiler.BundlePlatform{}, fmt.Errorf("unsupported platform %s", platformID)
			}
			return p, compiler.BundleVersion{}, compiler.BundlePlatform{}, fmt.Errorf("unknown version %s", versionID)
		}
	}
	return compiler.BundleProduct{}, compiler.BundleVersion{}, compiler.BundlePlatform{}, fmt.Errorf("unknown product %s", productID)
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

func activateLauncher(productRoot, command, entrypoint, binDir, previousLauncher string) (string, error) {
	if info, err := os.Stat(entrypoint); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("activation entrypoint is unavailable")
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	launcher := filepath.Join(binDir, command)
	if runtime.GOOS == "windows" {
		launcher += ".cmd"
	}
	if _, err := os.Lstat(launcher); err == nil && previousLauncher != launcher {
		return "", fmt.Errorf("refusing to overwrite foreign launcher %s", launcher)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporary := launcher + fmt.Sprintf(".taolu-%d", os.Getpid())
	_ = os.Remove(temporary)
	if runtime.GOOS == "windows" {
		body := "@echo off\r\n\"" + strings.ReplaceAll(entrypoint, "%", "%%") + "\" %*\r\n"
		if err := os.WriteFile(temporary, []byte(body), 0o600); err != nil {
			return "", err
		}
	} else if err := os.Symlink(entrypoint, temporary); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, launcher); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	owner := map[string]string{"schema": "taolu.launcher-owner/v1", "product": filepath.Base(productRoot), "launcher": launcher}
	if err := writeAtomic(filepath.Join(productRoot, "launcher.json"), owner); err != nil {
		return "", err
	}
	return launcher, nil
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
	if !safeSegment(state.Product) || !safeSegment(state.Command) || !safeSegment(state.Current) || (state.Previous != "" && !safeSegment(state.Previous)) {
		return state, errors.New("unsafe install state identity")
	}
	for version, entrypoint := range state.Entrypoints {
		clean := filepath.ToSlash(entrypoint)
		if !safeSegment(version) || entrypoint == "" || filepath.IsAbs(entrypoint) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || strings.ContainsAny(entrypoint, `\\:`) {
			return state, errors.New("unsafe install state entrypoint")
		}
	}
	return state, nil
}

func safeSegment(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\:`)
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
