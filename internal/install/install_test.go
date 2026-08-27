// SPDX-License-Identifier: Apache-2.0
package install

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
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
	bundle := compiler.Bundle{Schema: compiler.BundleSchema, TaoluVersion: "1.0.0", CatalogSHA256: digest, Products: []compiler.BundleProduct{{ID: "demo", Repository: "example/demo", ReleaseTag: "v" + version, ReleaseID: 1, Adapter: compiler.Adapter{Kind: "exact-asset", Version: 1}, Platforms: []compiler.BundlePlatform{{ID: "linux-x64", AssetID: 1, AssetName: "product.zip", URL: "file://" + archive, Size: int64(len(b)), SHA256: digest, Archive: "zip", StripComponents: 1, Entrypoint: "bin/tool"}}}}}
	rooted := rootBundle(t, bundle)
	return rooted, dir
}
func rootBundle(t *testing.T, b compiler.Bundle) compiler.Bundle {
	t.Helper()
	d := t.TempDir()
	p := b.Products[0].Platforms[0]
	cat := `{"schema":"taolu.catalog/v1","taoluVersion":"1.0.0","products":[{"id":"demo","repository":"example/demo","releaseTag":"` + b.Products[0].ReleaseTag + `","adapter":{"kind":"exact-asset","version":1},"platforms":[{"id":"linux-x64","assetName":"product.zip","sha256":"` + p.SHA256 + `","archive":"zip","stripComponents":1,"entrypoint":"bin/tool"}]}]}`
	rel := `{"schema":"taolu.github-releases/v1","releases":[{"repository":"example/demo","tag":"` + b.Products[0].ReleaseTag + `","id":1,"assets":[{"id":1,"name":"product.zip","url":"` + p.URL + `","size":` + strconv.FormatInt(p.Size, 10) + `}]}]}`
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
	if _, err := Install(b1, "demo", "linux-x64", root); err != nil {
		t.Fatal(err)
	}
	b2, _ := fixture(t, "1.1.0")
	if _, err := Install(b2, "demo", "linux-x64", root); err != nil {
		t.Fatal(err)
	}
	receipt, err := Rollback("demo", root)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != "1.0.0" {
		t.Fatalf("unexpected rollback %s", receipt.Version)
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
