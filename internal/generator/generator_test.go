// SPDX-License-Identifier: Apache-2.0
package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kungfu-systems/taolu/internal/compiler"
)

func TestGenerateDeterministicInstaller(t *testing.T) {
	root := filepath.Join("..", "..")
	a := t.TempDir()
	b := t.TempDir()
	for _, out := range []string{a, b} {
		bundleURL := "https://site.example/taolu/bundle.json"
		if _, err := Generate(filepath.Join(root, "testdata", "catalog.json"), filepath.Join(root, "testdata", "releases.json"), bundleURL, out); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"bundle.json", "install.sh", "install.ps1"} {
		x, err := os.ReadFile(filepath.Join(a, name))
		if err != nil {
			t.Fatal(err)
		}
		y, err := os.ReadFile(filepath.Join(b, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(x) != string(y) {
			t.Fatalf("%s is not deterministic", name)
		}
		if name != "bundle.json" && (!strings.Contains(string(x), "kungfu") || !strings.Contains(string(x), "sha256:")) {
			t.Fatalf("%s does not bind product and root", name)
		}
	}
}

func TestGenerateMultiProductVersionedInstaller(t *testing.T) {
	root := filepath.Join("..", "..")
	catalogPath := filepath.Join(root, "examples", "site-libkungfu-dev", "catalog.json")
	var catalog compiler.Catalog
	if _, err := compiler.ReadJSON(catalogPath, &catalog); err != nil {
		t.Fatal(err)
	}
	releases := compiler.Releases{Schema: "taolu.github-releases/v1"}
	var assetID int64 = 1
	for productIndex, product := range catalog.Products {
		for versionIndex, version := range product.Versions {
			release := compiler.Release{Repository: product.Repository, Tag: version.ReleaseTag, ID: int64((productIndex+1)*100 + versionIndex + 1)}
			seen := map[string]bool{}
			for _, platform := range version.Platforms {
				names := []string{platform.AssetName}
				for _, evidence := range platform.Evidence {
					if evidence.AssetName != "" {
						names = append(names, evidence.AssetName)
					}
				}
				for _, name := range names {
					if seen[name] {
						continue
					}
					seen[name] = true
					release.Assets = append(release.Assets, compiler.Asset{ID: assetID, Name: name, URL: "https://github.com/" + product.Repository + "/releases/download/" + version.ReleaseTag + "/" + name, Size: 1})
					assetID++
				}
			}
			releases.Releases = append(releases.Releases, release)
		}
	}
	releaseBytes, _ := json.Marshal(releases)
	releasesPath := filepath.Join(t.TempDir(), "releases.json")
	if err := os.WriteFile(releasesPath, releaseBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	bundle, err := Generate(catalogPath, releasesPath, "https://libkungfu.dev/install/v1/bundle.json", out)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Products) != 4 || bundle.DefaultProduct != "kungfu" {
		t.Fatalf("unexpected Site bundle shape: products=%d default=%s", len(bundle.Products), bundle.DefaultProduct)
	}
	for _, name := range []string{"install.sh", "install.ps1"} {
		body, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "all") || !strings.Contains(string(body), "--version") || !strings.Contains(string(body), "--rollback") {
			t.Fatalf("%s does not expose the multi-product versioned surface", name)
		}
	}
}

func TestGenerateRejectsExecutableBundleURL(t *testing.T) {
	root := filepath.Join("..", "..")
	_, err := Generate(filepath.Join(root, "testdata", "catalog.json"), filepath.Join(root, "testdata", "releases.json"), "https://site.example/bundle'$(touch bad)'", t.TempDir())
	if err == nil {
		t.Fatal("expected unsafe bundle URL rejection")
	}
}
