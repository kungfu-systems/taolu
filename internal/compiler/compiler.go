// SPDX-License-Identifier: Apache-2.0
package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const BundleSchema = "taolu.bundle/v1"

var (
	idPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	tagPattern        = regexp.MustCompile(`^v?[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type Catalog struct {
	Schema       string    `json:"schema"`
	TaoluVersion string    `json:"taoluVersion"`
	Products     []Product `json:"products"`
}

type Product struct {
	ID         string     `json:"id"`
	Repository string     `json:"repository"`
	ReleaseTag string     `json:"releaseTag"`
	Adapter    Adapter    `json:"adapter"`
	Platforms  []Platform `json:"platforms"`
}

type Adapter struct {
	Kind    string `json:"kind"`
	Version int    `json:"version"`
}

type Platform struct {
	ID              string `json:"id"`
	AssetName       string `json:"assetName"`
	SHA256          string `json:"sha256"`
	Archive         string `json:"archive"`
	StripComponents int    `json:"stripComponents"`
	Entrypoint      string `json:"entrypoint"`
}

type Releases struct {
	Schema   string    `json:"schema"`
	Releases []Release `json:"releases"`
}

type Release struct {
	Repository string  `json:"repository"`
	Tag        string  `json:"tag"`
	ID         int64   `json:"id"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type Bundle struct {
	Schema        string          `json:"schema"`
	TaoluVersion  string          `json:"taoluVersion"`
	CatalogSHA256 string          `json:"catalogSha256"`
	Products      []BundleProduct `json:"products"`
	BundleRoot    string          `json:"bundleRoot,omitempty"`
}

type BundleProduct struct {
	ID         string           `json:"id"`
	Repository string           `json:"repository"`
	ReleaseTag string           `json:"releaseTag"`
	ReleaseID  int64            `json:"releaseId"`
	Adapter    Adapter          `json:"adapter"`
	Platforms  []BundlePlatform `json:"platforms"`
}

type BundlePlatform struct {
	ID              string `json:"id"`
	AssetID         int64  `json:"assetId"`
	AssetName       string `json:"assetName"`
	URL             string `json:"url"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
	Archive         string `json:"archive"`
	StripComponents int    `json:"stripComponents"`
	Entrypoint      string `json:"entrypoint"`
}

func ReadJSON(path string, value any) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return b, nil
}

func Compile(catalogPath, releasesPath string) (Bundle, error) {
	var catalog Catalog
	catalogBytes, err := ReadJSON(catalogPath, &catalog)
	if err != nil {
		return Bundle{}, err
	}
	var releases Releases
	if _, err := ReadJSON(releasesPath, &releases); err != nil {
		return Bundle{}, err
	}
	if catalog.Schema != "taolu.catalog/v1" || releases.Schema != "taolu.github-releases/v1" {
		return Bundle{}, errors.New("unsupported catalog or release metadata schema")
	}
	if catalog.TaoluVersion == "" || len(catalog.Products) == 0 {
		return Bundle{}, errors.New("taoluVersion is required")
	}
	releaseIndex := map[string]Release{}
	for _, r := range releases.Releases {
		key := r.Repository + "@" + r.Tag
		if _, exists := releaseIndex[key]; exists {
			return Bundle{}, fmt.Errorf("ambiguous release %s", key)
		}
		releaseIndex[key] = r
	}
	bundle := Bundle{Schema: BundleSchema, TaoluVersion: catalog.TaoluVersion}
	catalogSum := sha256.Sum256(catalogBytes)
	bundle.CatalogSHA256 = hex.EncodeToString(catalogSum[:])
	seenProducts := map[string]bool{}
	for _, p := range catalog.Products {
		if !idPattern.MatchString(p.ID) || seenProducts[p.ID] {
			return Bundle{}, fmt.Errorf("invalid or duplicate product id %q", p.ID)
		}
		seenProducts[p.ID] = true
		if !repositoryPattern.MatchString(p.Repository) || !tagPattern.MatchString(p.ReleaseTag) {
			return Bundle{}, fmt.Errorf("invalid release coordinates for %s", p.ID)
		}
		if p.Adapter != (Adapter{Kind: "exact-asset", Version: 1}) {
			return Bundle{}, fmt.Errorf("product %s uses unqualified adapter", p.ID)
		}
		r, ok := releaseIndex[p.Repository+"@"+p.ReleaseTag]
		if !ok {
			return Bundle{}, fmt.Errorf("release not found for %s", p.ID)
		}
		bp := BundleProduct{ID: p.ID, Repository: p.Repository, ReleaseTag: p.ReleaseTag, ReleaseID: r.ID, Adapter: p.Adapter}
		seenPlatforms := map[string]bool{}
		if len(p.Platforms) == 0 {
			return Bundle{}, fmt.Errorf("product %s has no platforms", p.ID)
		}
		for _, platform := range p.Platforms {
			if !idPattern.MatchString(platform.ID) || seenPlatforms[platform.ID] {
				return Bundle{}, fmt.Errorf("invalid or duplicate platform %q", platform.ID)
			}
			seenPlatforms[platform.ID] = true
			if !validDigest(platform.SHA256) {
				return Bundle{}, fmt.Errorf("invalid sha256 for %s/%s", p.ID, platform.ID)
			}
			if platform.StripComponents < 0 || !validArchive(platform.Archive) || !safeRelative(platform.Entrypoint) || filepath.Base(platform.AssetName) != platform.AssetName || strings.ContainsAny(platform.AssetName, `/\\`) {
				return Bundle{}, fmt.Errorf("invalid extraction contract for %s/%s", p.ID, platform.ID)
			}
			matches := make([]Asset, 0, 1)
			for _, a := range r.Assets {
				if a.Name == platform.AssetName {
					matches = append(matches, a)
				}
			}
			if len(matches) != 1 {
				return Bundle{}, fmt.Errorf("asset %q resolved %d times", platform.AssetName, len(matches))
			}
			a := matches[0]
			if a.ID <= 0 || a.Size < 0 || !(strings.HasPrefix(a.URL, "https://") || strings.HasPrefix(a.URL, "file://")) {
				return Bundle{}, fmt.Errorf("invalid release asset %q", a.Name)
			}
			bp.Platforms = append(bp.Platforms, BundlePlatform{ID: platform.ID, AssetID: a.ID, AssetName: a.Name, URL: a.URL, Size: a.Size, SHA256: strings.ToLower(platform.SHA256), Archive: platform.Archive, StripComponents: platform.StripComponents, Entrypoint: platform.Entrypoint})
		}
		sort.Slice(bp.Platforms, func(i, j int) bool { return bp.Platforms[i].ID < bp.Platforms[j].ID })
		bundle.Products = append(bundle.Products, bp)
	}
	sort.Slice(bundle.Products, func(i, j int) bool { return bundle.Products[i].ID < bundle.Products[j].ID })
	root, err := bundleRoot(bundle)
	if err != nil {
		return Bundle{}, err
	}
	bundle.BundleRoot = root
	return bundle, nil
}

func Write(path string, bundle Bundle) error {
	b, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func VerifyBundle(bundle Bundle) error {
	if bundle.Schema != BundleSchema || bundle.BundleRoot == "" || bundle.TaoluVersion == "" || len(bundle.Products) == 0 {
		return errors.New("invalid bundle schema or root")
	}
	seenProducts := map[string]bool{}
	for _, product := range bundle.Products {
		if !idPattern.MatchString(product.ID) || seenProducts[product.ID] || !repositoryPattern.MatchString(product.Repository) || !tagPattern.MatchString(product.ReleaseTag) || product.ReleaseID <= 0 || product.Adapter != (Adapter{Kind: "exact-asset", Version: 1}) {
			return fmt.Errorf("invalid bundled product %q", product.ID)
		}
		seenProducts[product.ID] = true
		seenPlatforms := map[string]bool{}
		if len(product.Platforms) == 0 {
			return fmt.Errorf("bundled product %s has no platforms", product.ID)
		}
		for _, platform := range product.Platforms {
			if !idPattern.MatchString(platform.ID) || seenPlatforms[platform.ID] || platform.AssetID <= 0 || platform.Size < 0 || !validDigest(platform.SHA256) || !validArchive(platform.Archive) || platform.StripComponents < 0 || !safeRelative(platform.Entrypoint) || filepath.Base(platform.AssetName) != platform.AssetName || strings.ContainsAny(platform.AssetName, `/\\`) {
				return fmt.Errorf("invalid bundled platform %q", platform.ID)
			}
			if !(strings.HasPrefix(platform.URL, "https://") || strings.HasPrefix(platform.URL, "file://")) {
				return fmt.Errorf("invalid bundled asset URL for %q", platform.ID)
			}
			seenPlatforms[platform.ID] = true
		}
	}
	root, err := bundleRoot(bundle)
	if err != nil {
		return err
	}
	if root != bundle.BundleRoot {
		return errors.New("bundle root mismatch")
	}
	return nil
}

func bundleRoot(bundle Bundle) (string, error) {
	bundle.BundleRoot = ""
	b, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func validArchive(v string) bool { return v == "zip" || v == "tar.gz" || v == "file" }
func safeRelative(v string) bool {
	if v == "" || strings.ContainsAny(v, `\\:`) {
		return false
	}
	clean := filepath.ToSlash(v)
	return !filepath.IsAbs(v) && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}
