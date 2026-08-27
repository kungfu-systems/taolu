// SPDX-License-Identifier: Apache-2.0
package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileIsDeterministic(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	a, err := Compile(filepath.Join(root, "catalog.json"), filepath.Join(root, "releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile(filepath.Join(root, "catalog.json"), filepath.Join(root, "releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	if a.BundleRoot != b.BundleRoot {
		t.Fatalf("roots differ: %s %s", a.BundleRoot, b.BundleRoot)
	}
	if err := VerifyBundle(a); err != nil {
		t.Fatal(err)
	}
}

func TestCompileRejectsAmbiguousAsset(t *testing.T) {
	root := t.TempDir()
	catalog, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "catalog.json"))
	releaseBytes, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "releases.json"))
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), catalog, 0o644); err != nil {
		t.Fatal(err)
	}
	var releases Releases
	if err := json.Unmarshal(releaseBytes, &releases); err != nil {
		t.Fatal(err)
	}
	releases.Releases[0].Assets = append(releases.Releases[0].Assets, Asset{ID: 99, Name: "kungfu-linux-x64.tar.gz", URL: "https://example.invalid/duplicate", Size: 1})
	releaseBytes, _ = json.Marshal(releases)
	if err := os.WriteFile(filepath.Join(root, "releases.json"), releaseBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(filepath.Join(root, "catalog.json"), filepath.Join(root, "releases.json")); err == nil {
		t.Fatal("expected ambiguous asset failure")
	}
}

func TestCompileRejectsPathBearingIdentity(t *testing.T) {
	root := t.TempDir()
	catalogBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog Catalog
	if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog.Products[0].ID = "../foreign"
	catalogBytes, _ = json.Marshal(catalog)
	catalogPath := filepath.Join(root, "catalog.json")
	if err := os.WriteFile(catalogPath, catalogBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(catalogPath, filepath.Join("..", "..", "testdata", "releases.json")); err == nil {
		t.Fatal("expected path-bearing product id failure")
	}
}

func TestVerifyBundleRejectsRerootedPathBearingIdentity(t *testing.T) {
	bundle, err := Compile(filepath.Join("..", "..", "testdata", "catalog.json"), filepath.Join("..", "..", "testdata", "releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	bundle.Products[0].ID = "../foreign"
	bundle.BundleRoot, err = bundleRoot(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBundle(bundle); err == nil {
		t.Fatal("expected semantic bundle validation failure")
	}
}

func TestSiteCatalogExamplesCompile(t *testing.T) {
	for _, site := range []string{"site-kungfu-tech", "site-libkungfu-dev"} {
		t.Run(site, func(t *testing.T) {
			root := t.TempDir()
			examplePath := filepath.Join("..", "..", "examples", site, "catalog.json")
			catalogBytes, err := os.ReadFile(examplePath)
			if err != nil {
				t.Fatal(err)
			}
			catalogBytes = []byte(strings.ReplaceAll(string(catalogBytes), "REPLACE_WITH_RELEASE_SHA256", strings.Repeat("a", 64)))
			var catalog Catalog
			if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
				t.Fatal(err)
			}

			releases := Releases{Schema: "taolu.github-releases/v1"}
			for productIndex, product := range catalog.Products {
				release := Release{Repository: product.Repository, Tag: product.ReleaseTag, ID: int64(productIndex + 1)}
				for platformIndex, platform := range product.Platforms {
					release.Assets = append(release.Assets, Asset{
						ID:   int64((productIndex+1)*100 + platformIndex),
						Name: platform.AssetName,
						URL:  "https://example.invalid/" + platform.AssetName,
						Size: 1,
					})
				}
				releases.Releases = append(releases.Releases, release)
			}

			catalogPath := filepath.Join(root, "catalog.json")
			releasesPath := filepath.Join(root, "releases.json")
			if err := os.WriteFile(catalogPath, catalogBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			releasesBytes, err := json.Marshal(releases)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(releasesPath, releasesBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			bundle, err := Compile(catalogPath, releasesPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(bundle.Products) != len(catalog.Products) {
				t.Fatalf("compiled %d products, want %d", len(bundle.Products), len(catalog.Products))
			}
		})
	}
}
