// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetForPlatform(t *testing.T) {
	goos, goarch, err := targetForPlatform("windows-x64")
	if err != nil {
		t.Fatal(err)
	}
	if goos != "windows" || goarch != "amd64" {
		t.Fatalf("got %s/%s", goos, goarch)
	}
	if _, _, err := targetForPlatform("plan9-x64"); err == nil {
		t.Fatal("expected unsupported platform failure")
	}
}

func TestPackageBootstrapBindsVersion(t *testing.T) {
	out := filepath.Join(t.TempDir(), "install.sh")
	if err := packageBootstrap(filepath.Join("..", "..", "bootstrap", "install.sh"), out, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "@TAOLU_VERSION@") {
		t.Fatal("version placeholder was not bound")
	}
}
