// SPDX-License-Identifier: Apache-2.0
package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestGenerateRejectsExecutableBundleURL(t *testing.T) {
	root := filepath.Join("..", "..")
	_, err := Generate(filepath.Join(root, "testdata", "catalog.json"), filepath.Join(root, "testdata", "releases.json"), "https://site.example/bundle'$(touch bad)'", t.TempDir())
	if err == nil {
		t.Fatal("expected unsafe bundle URL rejection")
	}
}
