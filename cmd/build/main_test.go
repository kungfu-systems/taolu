// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

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
