// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	productmeta "github.com/kungfu-systems/taolu/internal/product"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	platform := os.Getenv("BUILDCHAIN_PLATFORM")
	if platform == "" {
		arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
		platform = runtime.GOOS + "-" + arch
	}
	goos, goarch, err := targetForPlatform(platform)
	if err != nil {
		return err
	}
	name := "taolu-" + platform
	if goos == "windows" {
		name += ".exe"
	}
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(".buildchain", 0o755); err != nil {
		return err
	}
	path := filepath.Join("dist", name)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", path, "./cmd/taolu")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	digest := hex.EncodeToString(sum[:])
	if err := os.WriteFile(path+".sha256", []byte(digest+"  "+name+"\n"), 0o644); err != nil {
		return err
	}
	if platform == "linux-x64" {
		if err := packageBootstrap("bootstrap/install.sh", "dist/taolu-install.sh", 0o755); err != nil {
			return err
		}
	}
	if platform == "windows-x64" {
		if err := packageBootstrap("bootstrap/install.ps1", "dist/taolu-install.ps1", 0o644); err != nil {
			return err
		}
	}
	receipt := map[string]any{"schema": "taolu.build-receipt/v1", "platform": platform, "artifact": path, "sha256": digest, "size": len(b)}
	out, _ := json.MarshalIndent(receipt, "", "  ")
	out = append(out, '\n')
	return os.WriteFile(".buildchain/build.receipt", out, 0o644)
}

func packageBootstrap(source, destination string, mode os.FileMode) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	b = []byte(strings.ReplaceAll(string(b), "@TAOLU_VERSION@", productmeta.Version))
	return os.WriteFile(destination, b, mode)
}

func targetForPlatform(platform string) (string, string, error) {
	targets := map[string][2]string{
		"darwin-arm64":  {"darwin", "arm64"},
		"darwin-x64":    {"darwin", "amd64"},
		"linux-arm64":   {"linux", "arm64"},
		"linux-x64":     {"linux", "amd64"},
		"windows-x64":   {"windows", "amd64"},
		"windows-arm64": {"windows", "arm64"},
	}
	target, ok := targets[platform]
	if !ok {
		return "", "", fmt.Errorf("unsupported Buildchain platform %q", platform)
	}
	return target[0], target[1], nil
}
