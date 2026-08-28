// SPDX-License-Identifier: Apache-2.0
package install

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kungfu-systems/taolu/internal/compiler"
)

func fixture(t *testing.T, version string) (compiler.Bundle, string) {
	t.Helper()
	dir := t.TempDir()
	archive := filepath.Join(dir, "product.zip")
	f, _ := os.Create(archive)
	w := zip.NewWriter(f)
	e, _ := w.Create("pkg/bin/tool")
	_, _ = e.Write([]byte(version))
	_ = w.Close()
	_ = f.Close()
	b, _ := os.ReadFile(archive)
	sum := sha256.Sum256(b)
	digest := hex.EncodeToString(sum[:])
	artifactPath := filepath.ToSlash(archive)
	if filepath.VolumeName(archive) != "" {
		artifactPath = "/" + artifactPath
	}
	artifactURL := (&url.URL{Scheme: "file", Path: artifactPath}).String()
	bundle := compiler.Bundle{Schema: compiler.BundleSchema, TaoluVersion: "1.0.0", CatalogSHA256: digest, Products: []compiler.BundleProduct{{ID: "demo", Repository: "example/demo", Adapter: compiler.Adapter{Kind: "exact-asset", Version: 1}, Versions: []compiler.BundleVersion{{Version: version, ReleaseTag: "v" + version, ReleaseID: 1, Platforms: []compiler.BundlePlatform{{ID: "linux-x64", AssetID: 1, AssetName: "product.zip", URL: artifactURL, Size: int64(len(b)), SHA256: digest, Archive: "zip", StripComponents: 1, Entrypoint: "bin/tool"}}}}}}}
	rooted := rootBundle(t, bundle)
	return rooted, dir
}
func rootBundle(t *testing.T, b compiler.Bundle) compiler.Bundle {
	t.Helper()
	d := t.TempDir()
	v := b.Products[0].Versions[0]
	p := v.Platforms[0]
	cat := `{"schema":"taolu.catalog/v1","taoluVersion":"1.0.0","products":[{"id":"demo","repository":"example/demo","releaseTag":` + strconv.Quote(v.ReleaseTag) + `,"adapter":{"kind":"exact-asset","version":1},"platforms":[{"id":"linux-x64","assetName":"product.zip","sha256":` + strconv.Quote(p.SHA256) + `,"archive":"zip","stripComponents":1,"entrypoint":"bin/tool"}]}]}`
	rel := `{"schema":"taolu.github-releases/v1","releases":[{"repository":"example/demo","tag":` + strconv.Quote(v.ReleaseTag) + `,"id":1,"assets":[{"id":1,"name":"product.zip","url":` + strconv.Quote(p.URL) + `,"size":` + strconv.FormatInt(p.Size, 10) + `}]}]}`
	_ = os.WriteFile(filepath.Join(d, "c.json"), []byte(cat), 0o644)
	_ = os.WriteFile(filepath.Join(d, "r.json"), []byte(rel), 0o644)
	out, err := compiler.Compile(filepath.Join(d, "c.json"), filepath.Join(d, "r.json"))
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestInstallAndRollback(t *testing.T) {
	b1, _ := fixture(t, "1.0.0")
	root := t.TempDir()
	first, err := Install(b1, "demo", "linux-x64", root)
	if err != nil {
		t.Fatal(err)
	}
	assertLauncherTarget(t, root, filepath.Join(first.InstalledPath, "bin", "tool"))
	if receipt, err := Install(b1, "demo", "linux-x64", root); err != nil || receipt.Operation != "activate" {
		t.Fatalf("exact reinstall is not idempotent: receipt=%+v err=%v", receipt, err)
	}
	b2, _ := fixture(t, "1.1.0")
	second, err := Install(b2, "demo", "linux-x64", root)
	if err != nil {
		t.Fatal(err)
	}
	assertLauncherTarget(t, root, filepath.Join(second.InstalledPath, "bin", "tool"))
	receipt, err := Rollback("demo", root)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != "1.0.0" {
		t.Fatalf("unexpected rollback %s", receipt.Version)
	}
	assertLauncherTarget(t, root, filepath.Join(first.InstalledPath, "bin", "tool"))
}

func assertLauncherTarget(t *testing.T, root, target string) {
	t.Helper()
	launcher := filepath.Join(root, "bin", "demo")
	if runtime.GOOS == "windows" {
		launcher += ".cmd"
		body, err := os.ReadFile(launcher)
		expected := "@echo off\r\n\"" + strings.ReplaceAll(target, "%", "%%") + "\" %*\r\n"
		if err != nil || string(body) != expected {
			t.Fatalf("launcher does not activate %q: body=%q err=%v", target, body, err)
		}
		return
	}
	actual, err := os.Readlink(launcher)
	if err != nil || actual != target {
		t.Fatalf("launcher does not activate %q: target=%q err=%v", target, actual, err)
	}
}
func TestOwnershipConflict(t *testing.T) {
	b, _ := fixture(t, "1.0.0")
	root := t.TempDir()
	foreign := filepath.Join(root, "products", "demo")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "foreign"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(b, "demo", "linux-x64", root); err == nil {
		t.Fatal("expected ownership conflict")
	}
}

func TestDigestMismatchAndInterruptedActivationFailClosed(t *testing.T) {
	bundle, artifactRoot := fixture(t, "1.0.0")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(artifactRoot, "product.zip"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(bundle, "demo", "linux-x64", root); err == nil {
		t.Fatal("expected digest mismatch")
	}
	bundle, _ = fixture(t, "1.0.0")
	productRoot := filepath.Join(root, "products", "demo")
	if err := ensureOwnership(productRoot, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(productRoot, "versions", ".taolu-stage-crashed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(bundle, "demo", "linux-x64", root); err == nil {
		t.Fatal("expected interrupted activation failure")
	}
}

func TestExactVersionAndEvidenceSelection(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "demo-1.0.0")
	evidencePath := filepath.Join(root, "demo-1.0.0.provenance.json")
	if err := os.WriteFile(assetPath, []byte("version-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("evidence-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	assetBytes, _ := os.ReadFile(assetPath)
	evidenceBytes, _ := os.ReadFile(evidencePath)
	assetDigest := sha256.Sum256(assetBytes)
	evidenceDigest := sha256.Sum256(evidenceBytes)
	fileURL := func(path string) string {
		path = filepath.ToSlash(path)
		if filepath.VolumeName(path) != "" {
			path = "/" + path
		}
		return (&url.URL{Scheme: "file", Path: path}).String()
	}
	catalog := compiler.Catalog{Schema: "taolu.catalog/v1", TaoluVersion: "1.0.0", DefaultProduct: "demo", Products: []compiler.Product{{ID: "demo", Command: "demo", Repository: "example/demo", DefaultVersion: "1.0.0", Adapter: compiler.Adapter{Kind: "exact-asset", Version: 1}, Versions: []compiler.Version{{Version: "1.0.0", ReleaseTag: "v1.0.0", Platforms: []compiler.Platform{{ID: PlatformID(), AssetName: filepath.Base(assetPath), SHA256: hex.EncodeToString(assetDigest[:]), Archive: "file", Entrypoint: "demo", EntrypointSHA256: hex.EncodeToString(assetDigest[:]), Evidence: []compiler.EvidenceAsset{{Kind: "provenance", AssetName: filepath.Base(evidencePath), SHA256: hex.EncodeToString(evidenceDigest[:])}}}}}}}}}
	releases := compiler.Releases{Schema: "taolu.github-releases/v1", Releases: []compiler.Release{{Repository: "example/demo", Tag: "v1.0.0", ID: 1, Assets: []compiler.Asset{{ID: 1, Name: filepath.Base(assetPath), URL: fileURL(assetPath), Size: int64(len(assetBytes))}, {ID: 2, Name: filepath.Base(evidencePath), URL: fileURL(evidencePath), Size: int64(len(evidenceBytes))}}}}}
	catalogBytes, _ := json.Marshal(catalog)
	releaseBytes, _ := json.Marshal(releases)
	catalogPath := filepath.Join(root, "catalog.json")
	releasesPath := filepath.Join(root, "releases.json")
	_ = os.WriteFile(catalogPath, catalogBytes, 0o644)
	_ = os.WriteFile(releasesPath, releaseBytes, 0o644)
	bundle, err := compiler.Compile(catalogPath, releasesPath)
	if err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(root, "home")
	receipt, err := InstallWithOptions(bundle, Options{Product: "demo", Version: "1.0.0", Root: installRoot})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != "1.0.0" {
		t.Fatalf("selected %s", receipt.Version)
	}

	otherRoot := filepath.Join(root, "tampered-home")
	if err := os.WriteFile(evidencePath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallWithOptions(bundle, Options{Product: "demo", Version: "1.0.0", Root: otherRoot}); err == nil {
		t.Fatal("expected evidence digest mismatch")
	}
}
