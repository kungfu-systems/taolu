// SPDX-License-Identifier: Apache-2.0
package product

import (
	_ "embed"
	"encoding/json"
)

//go:embed version.json
var versionBytes []byte

var Version = readVersion()

func readVersion() string {
	var document struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(versionBytes, &document); err != nil || document.Version == "" {
		panic("invalid embedded Taolu version")
	}
	return document.Version
}
