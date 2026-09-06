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

func TestBuildRejectsInvalidWorkflowPlatformBeforeHostFallback(t *testing.T) {
	t.Setenv("BUILDCHAIN_PLATFORM_ID", "unsupported-workflow-target")
	t.Setenv("BUILDCHAIN_PLATFORM", "linux-x64")
	if err := run(); err == nil || !strings.Contains(err.Error(), "unsupported-workflow-target") {
		t.Fatalf("expected the declared workflow platform to be validated, got %v", err)
	}
}

func TestBuildRejectsInvalidActionPlatformBeforeLegacyFallback(t *testing.T) {
	t.Setenv("BUILDCHAIN_PLATFORM_ID", "")
	t.Setenv("INPUT_PLATFORM-ID", "unsupported-action-target")
	t.Setenv("BUILDCHAIN_PLATFORM", "linux-x64")
	if err := run(); err == nil || !strings.Contains(err.Error(), "unsupported-action-target") {
		t.Fatalf("expected the declared action platform to be validated, got %v", err)
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
