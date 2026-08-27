// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kungfu-systems/taolu/internal/compiler"
	"github.com/kungfu-systems/taolu/internal/generator"
	installer "github.com/kungfu-systems/taolu/internal/install"
	productmeta "github.com/kungfu-systems/taolu/internal/product"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "taolu:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: taolu <installer|compile|install|rollback|status|version>")
	}
	switch args[0] {
	case "version":
		fmt.Println(productmeta.Version)
		return nil
	case "compile":
		fs := flag.NewFlagSet("compile", flag.ContinueOnError)
		catalog := fs.String("catalog", "", "site catalog JSON")
		releases := fs.String("releases", "", "pinned GitHub Release metadata JSON")
		out := fs.String("out", "bundle.json", "output bundle")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *catalog == "" || *releases == "" {
			return errors.New("--catalog and --releases are required")
		}
		bundle, err := compiler.Compile(*catalog, *releases)
		if err != nil {
			return err
		}
		if err = compiler.Write(*out, bundle); err != nil {
			return err
		}
		return emit(bundle)
	case "installer":
		fs := flag.NewFlagSet("installer", flag.ContinueOnError)
		config := fs.String("config", "", "Site-owned taolu.catalog/v1 JSON")
		releases := fs.String("releases", "", "pinned GitHub Release metadata JSON")
		bundleURL := fs.String("bundle-url", "", "final HTTPS or rehearsal file URL for bundle.json")
		out := fs.String("out", "installer", "output installer directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *config == "" || *releases == "" {
			return errors.New("--config and --releases are required")
		}
		bundle, err := generator.Generate(*config, *releases, *bundleURL, *out)
		if err != nil {
			return err
		}
		return emit(bundle)
	case "install":
		fs := flag.NewFlagSet("install", flag.ContinueOnError)
		bundlePath := fs.String("bundle", "", "compiled bundle JSON")
		product := fs.String("product", "", "product id")
		platform := fs.String("platform", "auto", "platform id")
		bundleRoot := fs.String("bundle-root", "", "expected sha256 bundle root")
		root := fs.String("root", defaultRoot(), "Taolu root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *bundlePath == "" || *product == "" {
			return errors.New("--bundle and --product are required")
		}
		var bundle compiler.Bundle
		if _, err := compiler.ReadJSON(*bundlePath, &bundle); err != nil {
			return err
		}
		if *bundleRoot != "" && bundle.BundleRoot != *bundleRoot {
			return errors.New("bundle root does not match the expected Site root")
		}
		if bundle.TaoluVersion != productmeta.Version {
			return fmt.Errorf("bundle requires Taolu %s, running %s", bundle.TaoluVersion, productmeta.Version)
		}
		receipt, err := installer.Install(bundle, *product, *platform, *root)
		if err != nil {
			return err
		}
		return emit(receipt)
	case "rollback":
		fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
		product := fs.String("product", "", "product id")
		root := fs.String("root", defaultRoot(), "Taolu root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *product == "" {
			return errors.New("--product is required")
		}
		receipt, err := installer.Rollback(*product, *root)
		if err != nil {
			return err
		}
		return emit(receipt)
	case "status":
		fs := flag.NewFlagSet("status", flag.ContinueOnError)
		product := fs.String("product", "", "product id")
		root := fs.String("root", defaultRoot(), "Taolu root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *product == "" {
			return errors.New("--product is required")
		}
		state, err := installer.Status(*product, *root)
		if err != nil {
			return err
		}
		return emit(state)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func emit(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
func defaultRoot() string {
	if v := os.Getenv("TAOLU_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".taolu")
}
