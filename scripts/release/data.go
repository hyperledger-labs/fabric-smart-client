/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Report is the part of `node version --json` the release data reads.
type Report struct {
	Components []Component `json:"components"`
	Stores     []Store     `json:"stores"`
}

// Component is the range of wire format versions a release supports for one component.
type Component struct {
	Name string `json:"name"`
	Min  uint32 `json:"min"`
	Max  uint32 `json:"max"`
}

// Store is the schema of one store type in one SQL dialect.
type Store struct {
	Type    string `json:"type"`
	Dialect string `json:"dialect"`
	Steps   []Step `json:"steps"`
}

// Step is one schema step. Contract is true when rolling back past it is a restore.
type Step struct {
	Version  int    `json:"version"`
	Name     string `json:"name"`
	Hash     string `json:"hash"`
	Contract bool   `json:"contract"`
}

// Data is the release data file docs/upgrade/releases/<version>.json. Tooling reads it, so its
// field names are fixed: fields are added, never renamed or removed.
type Data struct {
	Version    string      `json:"version"`
	Modules    []string    `json:"modules"`
	Upgrade    Upgrade     `json:"upgrade"`
	Stores     []Store     `json:"stores"`
	Components []Component `json:"components"`
	// DroppedNext lists the versions the next minor stops supporting.
	DroppedNext []string `json:"dropped_next"`
	// TestedWith maps a dependency to the version the release's tests ran against.
	TestedWith map[string]string `json:"tested_with"`
}

// Upgrade says how a deployment upgrades to the release and where the upgrade guide is.
type Upgrade struct {
	// Mode is stop-all: every node of a network stops before any runs the release.
	Mode  string `json:"mode"`
	Guide string `json:"guide"`
}

// guidePath returns the upgrade guide from prev to v, relative to the repository root.
func guidePath(prev, v semver) string {
	return fmt.Sprintf("docs/upgrade/v%d.%d-to-v%d.%d.md", prev.Major, prev.Minor, v.Major, v.Minor)
}

// newData returns the release data of version v of the repository in dir, with the stores and
// components of report r.
func newData(dir string, v semver, r Report) (Data, error) {
	// A patch has no upgrade guide of its own; its data and notes come with release branches.
	if v.Patch != 0 {
		return Data{}, errors.Errorf("%s: patch releases are not supported yet; release the next minor", v)
	}
	tags, err := releaseTags(dir)
	if err != nil {
		return Data{}, err
	}
	prev, err := previousRelease(tags, v)
	if err != nil {
		return Data{}, err
	}
	mods, err := modules(dir, v)
	if err != nil {
		return Data{}, err
	}
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return Data{}, errors.Wrap(err, "failed reading go.mod")
	}
	makefile, err := os.ReadFile(filepath.Join(dir, "Makefile"))
	if err != nil {
		return Data{}, errors.Wrap(err, "failed reading the Makefile")
	}
	tested, err := testedWith(gomod, makefile)
	if err != nil {
		return Data{}, err
	}
	return Data{
		Version:     v.String(),
		Modules:     mods,
		Upgrade:     Upgrade{Mode: "stop-all", Guide: guidePath(prev, v)},
		Stores:      r.Stores,
		Components:  r.Components,
		DroppedNext: []string{},
		TestedWith:  tested,
	}, nil
}

var (
	goDirective = regexp.MustCompile(`(?m)^go (\S+)$`)
	// pinLine is a version pin of the Makefile, as TestImagePins in integration/nwo reads it.
	pinLine = regexp.MustCompile(`(?m)^([A-Z0-9_]+_VERSION) \?= (\S+)$`)
	// testedPins maps the Makefile pins a node runs against to their tested_with key. The
	// monitoring and lint pins are test tooling and stay out.
	testedPins = map[string]string{
		"FABRIC_VERSION":             "fabric",
		"FABRIC_X_COMMITTER_VERSION": "fabric_x_committer",
		"FABRIC_X_TOOLS_VERSION":     "fabric_x_tools",
		"POSTGRES_VERSION":           "postgres",
	}
)

// testedWith returns tested_with from go.mod and the Makefile. Postgres drops the image
// suffix: 16.2-alpine is 16.2.
func testedWith(gomod, makefile []byte) (map[string]string, error) {
	m := goDirective.FindSubmatch(gomod)
	if m == nil {
		return nil, errors.New("go.mod has no go directive")
	}
	out := map[string]string{"go": string(m[1])}
	for _, p := range pinLine.FindAllSubmatch(makefile, -1) {
		if key, ok := testedPins[string(p[1])]; ok {
			out[key] = string(p[2])
		}
	}
	var missing []string
	for pin, key := range testedPins {
		if _, ok := out[key]; !ok {
			missing = append(missing, pin)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, errors.Errorf("the Makefile pins no %s", strings.Join(missing, ", "))
	}
	out["postgres"], _, _ = strings.Cut(out["postgres"], "-")
	return out, nil
}

// render returns d as the data file: two-space indent, map keys sorted, trailing newline.
func (d Data) render() ([]byte, error) {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling the release data")
	}
	return append(raw, '\n'), nil
}
