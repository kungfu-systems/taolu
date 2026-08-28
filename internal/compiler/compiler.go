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
	Schema         string    `json:"schema"`
	TaoluVersion   string    `json:"taoluVersion"`
	DefaultProduct string    `json:"defaultProduct,omitempty"`
	Products       []Product `json:"products"`
}

type Product struct {
	ID             string    `json:"id"`
	Command        string    `json:"command,omitempty"`
	Repository     string    `json:"repository"`
	DefaultVersion string    `json:"defaultVersion,omitempty"`
	Versions       []Version `json:"versions,omitempty"`
	// ReleaseTag and Platforms keep the pre-alpha single-version catalog
	// readable. Compile normalizes them into Versions before rooting a bundle.
	ReleaseTag string     `json:"releaseTag,omitempty"`
	Adapter    Adapter    `json:"adapter"`
	Platforms  []Platform `json:"platforms,omitempty"`
}

type Version struct {
	Version    string     `json:"version"`
	ReleaseTag string     `json:"releaseTag"`
	SourceSHA  string     `json:"sourceSha,omitempty"`
	Platforms  []Platform `json:"platforms"`
}

type Adapter struct {
	Kind    string `json:"kind"`
	Version int    `json:"version"`
}

type Platform struct {
	ID               string          `json:"id"`
	AssetName        string          `json:"assetName"`
	SHA256           string          `json:"sha256"`
	Archive          string          `json:"archive"`
	StripComponents  int             `json:"stripComponents"`
	Entrypoint       string          `json:"entrypoint"`
	EntrypointSHA256 string          `json:"entrypointSha256,omitempty"`
	Evidence         []EvidenceAsset `json:"evidence,omitempty"`
}

type EvidenceAsset struct {
	Kind      string `json:"kind"`
	AssetName string `json:"assetName,omitempty"`
	URL       string `json:"url,omitempty"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256"`
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
	Schema         string          `json:"schema"`
	TaoluVersion   string          `json:"taoluVersion"`
	DefaultProduct string          `json:"defaultProduct"`
	CatalogSHA256  string          `json:"catalogSha256"`
	Products       []BundleProduct `json:"products"`
	BundleRoot     string          `json:"bundleRoot,omitempty"`
}

type BundleProduct struct {
	ID             string          `json:"id"`
	Command        string          `json:"command"`
	Repository     string          `json:"repository"`
	DefaultVersion string          `json:"defaultVersion"`
	Adapter        Adapter         `json:"adapter"`
	Versions       []BundleVersion `json:"versions"`
}

type BundleVersion struct {
	Version    string           `json:"version"`
	ReleaseTag string           `json:"releaseTag"`
	ReleaseID  int64            `json:"releaseId"`
	SourceSHA  string           `json:"sourceSha,omitempty"`
	Platforms  []BundlePlatform `json:"platforms"`
}

type BundlePlatform struct {
	ID               string                `json:"id"`
	AssetID          int64                 `json:"assetId"`
	AssetName        string                `json:"assetName"`
	URL              string                `json:"url"`
	Size             int64                 `json:"size"`
	SHA256           string                `json:"sha256"`
	Archive          string                `json:"archive"`
	StripComponents  int                   `json:"stripComponents"`
	Entrypoint       string                `json:"entrypoint"`
	EntrypointSHA256 string                `json:"entrypointSha256,omitempty"`
	Evidence         []BundleEvidenceAsset `json:"evidence,omitempty"`
}

type BundleEvidenceAsset struct {
	Kind      string `json:"kind"`
	AssetID   int64  `json:"assetId"`
	AssetName string `json:"assetName"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
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
	bundle := Bundle{Schema: BundleSchema, TaoluVersion: catalog.TaoluVersion, DefaultProduct: catalog.DefaultProduct}
	catalogSum := sha256.Sum256(catalogBytes)
	bundle.CatalogSHA256 = hex.EncodeToString(catalogSum[:])
	seenProducts := map[string]bool{}
	seenCommands := map[string]bool{}
	for _, p := range catalog.Products {
		if !idPattern.MatchString(p.ID) || seenProducts[p.ID] {
			return Bundle{}, fmt.Errorf("invalid or duplicate product id %q", p.ID)
		}
		seenProducts[p.ID] = true
		if !repositoryPattern.MatchString(p.Repository) {
			return Bundle{}, fmt.Errorf("invalid repository for %s", p.ID)
		}
		if p.Adapter != (Adapter{Kind: "exact-asset", Version: 1}) {
			return Bundle{}, fmt.Errorf("product %s uses unqualified adapter", p.ID)
		}
		command := p.Command
		if command == "" {
			command = p.ID
		}
		if !idPattern.MatchString(command) || seenCommands[command] {
			return Bundle{}, fmt.Errorf("invalid command for %s", p.ID)
		}
		seenCommands[command] = true
		versions, defaultVersion, err := normalizedVersions(p)
		if err != nil {
			return Bundle{}, err
		}
		bp := BundleProduct{ID: p.ID, Command: command, Repository: p.Repository, DefaultVersion: defaultVersion, Adapter: p.Adapter}
		seenVersions := map[string]bool{}
		for _, version := range versions {
			if !idPattern.MatchString(version.Version) || !tagPattern.MatchString(version.ReleaseTag) || seenVersions[version.Version] {
				return Bundle{}, fmt.Errorf("invalid or duplicate version %q for %s", version.Version, p.ID)
			}
			seenVersions[version.Version] = true
			if version.SourceSHA != "" && !validGitSHA(version.SourceSHA) {
				return Bundle{}, fmt.Errorf("invalid sourceSha for %s@%s", p.ID, version.Version)
			}
			r, ok := releaseIndex[p.Repository+"@"+version.ReleaseTag]
			if !ok {
				return Bundle{}, fmt.Errorf("release not found for %s@%s", p.ID, version.Version)
			}
			bv := BundleVersion{Version: version.Version, ReleaseTag: version.ReleaseTag, ReleaseID: r.ID, SourceSHA: version.SourceSHA}
			seenPlatforms := map[string]bool{}
			if len(version.Platforms) == 0 {
				return Bundle{}, fmt.Errorf("product %s@%s has no platforms", p.ID, version.Version)
			}
			for _, platform := range version.Platforms {
				if !idPattern.MatchString(platform.ID) || seenPlatforms[platform.ID] {
					return Bundle{}, fmt.Errorf("invalid or duplicate platform %q", platform.ID)
				}
				seenPlatforms[platform.ID] = true
				if !validDigest(platform.SHA256) || (platform.EntrypointSHA256 != "" && !validDigest(platform.EntrypointSHA256)) {
					return Bundle{}, fmt.Errorf("invalid digest for %s@%s/%s", p.ID, version.Version, platform.ID)
				}
				if platform.StripComponents < 0 || !validArchive(platform.Archive) || !safeRelative(platform.Entrypoint) || !safeAssetName(platform.AssetName) {
					return Bundle{}, fmt.Errorf("invalid extraction contract for %s@%s/%s", p.ID, version.Version, platform.ID)
				}
				a, err := exactAsset(r, platform.AssetName)
				if err != nil {
					return Bundle{}, err
				}
				bundled := BundlePlatform{ID: platform.ID, AssetID: a.ID, AssetName: a.Name, URL: a.URL, Size: a.Size, SHA256: strings.ToLower(platform.SHA256), Archive: platform.Archive, StripComponents: platform.StripComponents, Entrypoint: platform.Entrypoint, EntrypointSHA256: strings.ToLower(platform.EntrypointSHA256)}
				seenEvidence := map[string]bool{}
				for _, evidence := range platform.Evidence {
					if !idPattern.MatchString(evidence.Kind) || !validDigest(evidence.SHA256) || seenEvidence[evidence.Kind] {
						return Bundle{}, fmt.Errorf("invalid or duplicate evidence for %s@%s/%s", p.ID, version.Version, platform.ID)
					}
					seenEvidence[evidence.Kind] = true
					var ea Asset
					if evidence.AssetName != "" && evidence.URL == "" {
						ea, err = exactAsset(r, evidence.AssetName)
						if err != nil {
							return Bundle{}, err
						}
					} else if evidence.AssetName == "" && evidence.URL != "" && evidence.Size >= 0 && strings.HasPrefix(evidence.URL, "https://") {
						ea = Asset{Name: filepath.Base(evidence.URL), URL: evidence.URL, Size: evidence.Size}
						if !safeAssetName(ea.Name) {
							return Bundle{}, fmt.Errorf("invalid evidence URL for %s@%s/%s", p.ID, version.Version, platform.ID)
						}
					} else {
						return Bundle{}, fmt.Errorf("evidence for %s@%s/%s must select one release asset or one exact HTTPS URL", p.ID, version.Version, platform.ID)
					}
					bundled.Evidence = append(bundled.Evidence, BundleEvidenceAsset{Kind: evidence.Kind, AssetID: ea.ID, AssetName: ea.Name, URL: ea.URL, Size: ea.Size, SHA256: strings.ToLower(evidence.SHA256)})
				}
				sort.Slice(bundled.Evidence, func(i, j int) bool { return bundled.Evidence[i].Kind < bundled.Evidence[j].Kind })
				bv.Platforms = append(bv.Platforms, bundled)
			}
			sort.Slice(bv.Platforms, func(i, j int) bool { return bv.Platforms[i].ID < bv.Platforms[j].ID })
			bp.Versions = append(bp.Versions, bv)
		}
		sort.Slice(bp.Versions, func(i, j int) bool { return bp.Versions[i].Version < bp.Versions[j].Version })
		bundle.Products = append(bundle.Products, bp)
	}
	sort.Slice(bundle.Products, func(i, j int) bool { return bundle.Products[i].ID < bundle.Products[j].ID })
	if bundle.DefaultProduct == "" {
		bundle.DefaultProduct = bundle.Products[0].ID
	}
	if !seenProducts[bundle.DefaultProduct] {
		return Bundle{}, fmt.Errorf("defaultProduct %q does not exist", bundle.DefaultProduct)
	}
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

func normalizedVersions(product Product) ([]Version, string, error) {
	if len(product.Versions) > 0 {
		if product.ReleaseTag != "" || len(product.Platforms) > 0 {
			return nil, "", fmt.Errorf("product %s mixes legacy and versioned release declarations", product.ID)
		}
		defaultVersion := product.DefaultVersion
		if defaultVersion == "" && len(product.Versions) == 1 {
			defaultVersion = product.Versions[0].Version
		}
		if defaultVersion == "" {
			return nil, "", fmt.Errorf("product %s requires defaultVersion", product.ID)
		}
		found := false
		for _, version := range product.Versions {
			if version.Version == defaultVersion {
				found = true
			}
		}
		if !found {
			return nil, "", fmt.Errorf("product %s defaultVersion %q does not exist", product.ID, defaultVersion)
		}
		return product.Versions, defaultVersion, nil
	}
	if product.ReleaseTag == "" || len(product.Platforms) == 0 {
		return nil, "", fmt.Errorf("product %s has no versions", product.ID)
	}
	version := strings.TrimPrefix(product.ReleaseTag, "v")
	return []Version{{Version: version, ReleaseTag: product.ReleaseTag, Platforms: product.Platforms}}, version, nil
}

func exactAsset(release Release, name string) (Asset, error) {
	matches := make([]Asset, 0, 1)
	for _, asset := range release.Assets {
		if asset.Name == name {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return Asset{}, fmt.Errorf("asset %q resolved %d times", name, len(matches))
	}
	asset := matches[0]
	if asset.ID <= 0 || asset.Size < 0 || !(strings.HasPrefix(asset.URL, "https://") || strings.HasPrefix(asset.URL, "file://")) {
		return Asset{}, fmt.Errorf("invalid release asset %q", asset.Name)
	}
	if strings.HasPrefix(asset.URL, "https://") {
		expected := "https://github.com/" + release.Repository + "/releases/download/" + release.Tag + "/" + asset.Name
		if asset.URL != expected {
			return Asset{}, fmt.Errorf("release asset %q escapes exact GitHub Release", asset.Name)
		}
	}
	return asset, nil
}

func VerifyBundle(bundle Bundle) error {
	if bundle.Schema != BundleSchema || bundle.BundleRoot == "" || bundle.TaoluVersion == "" || bundle.DefaultProduct == "" || len(bundle.Products) == 0 {
		return errors.New("invalid bundle schema or root")
	}
	seenProducts := map[string]bool{}
	seenCommands := map[string]bool{}
	for _, product := range bundle.Products {
		if !idPattern.MatchString(product.ID) || !idPattern.MatchString(product.Command) || seenProducts[product.ID] || seenCommands[product.Command] || !repositoryPattern.MatchString(product.Repository) || !idPattern.MatchString(product.DefaultVersion) || product.Adapter != (Adapter{Kind: "exact-asset", Version: 1}) || len(product.Versions) == 0 {
			return fmt.Errorf("invalid bundled product %q", product.ID)
		}
		seenProducts[product.ID] = true
		seenCommands[product.Command] = true
		seenVersions := map[string]bool{}
		defaultFound := false
		for _, version := range product.Versions {
			if !idPattern.MatchString(version.Version) || !tagPattern.MatchString(version.ReleaseTag) || version.ReleaseID <= 0 || seenVersions[version.Version] || (version.SourceSHA != "" && !validGitSHA(version.SourceSHA)) || len(version.Platforms) == 0 {
				return fmt.Errorf("invalid bundled version %q", version.Version)
			}
			seenVersions[version.Version] = true
			defaultFound = defaultFound || version.Version == product.DefaultVersion
			seenPlatforms := map[string]bool{}
			for _, platform := range version.Platforms {
				if !idPattern.MatchString(platform.ID) || seenPlatforms[platform.ID] || platform.AssetID <= 0 || platform.Size < 0 || !validDigest(platform.SHA256) || (platform.EntrypointSHA256 != "" && !validDigest(platform.EntrypointSHA256)) || !validArchive(platform.Archive) || platform.StripComponents < 0 || !safeRelative(platform.Entrypoint) || !safeAssetName(platform.AssetName) {
					return fmt.Errorf("invalid bundled platform %q", platform.ID)
				}
				if !(strings.HasPrefix(platform.URL, "https://") || strings.HasPrefix(platform.URL, "file://")) {
					return fmt.Errorf("invalid bundled asset URL for %q", platform.ID)
				}
				seenEvidence := map[string]bool{}
				for _, evidence := range platform.Evidence {
					if !idPattern.MatchString(evidence.Kind) || seenEvidence[evidence.Kind] || evidence.AssetID < 0 || evidence.Size < 0 || !safeAssetName(evidence.AssetName) || !validDigest(evidence.SHA256) || !(strings.HasPrefix(evidence.URL, "https://") || strings.HasPrefix(evidence.URL, "file://")) {
						return fmt.Errorf("invalid bundled evidence for %q", platform.ID)
					}
					seenEvidence[evidence.Kind] = true
				}
				seenPlatforms[platform.ID] = true
			}
		}
		if !defaultFound {
			return fmt.Errorf("bundled product %s default version is unavailable", product.ID)
		}
	}
	if !seenProducts[bundle.DefaultProduct] {
		return errors.New("default product is unavailable")
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
func validGitSHA(v string) bool {
	return len(v) == 40 && strings.IndexFunc(v, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdefABCDEF", r)
	}) == -1
}
func safeAssetName(v string) bool {
	return v != "" && filepath.Base(v) == v && !strings.ContainsAny(v, `/\\`)
}
func validArchive(v string) bool { return v == "zip" || v == "tar.gz" || v == "file" }
func safeRelative(v string) bool {
	if v == "" || strings.ContainsAny(v, `\\:`) {
		return false
	}
	clean := filepath.ToSlash(v)
	return !filepath.IsAbs(v) && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}
