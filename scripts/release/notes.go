/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// mainRepo is the GitHub repository of FSC releases, owner/name.
const mainRepo = "hyperledger-labs/fabric-smart-client"

// testedNames are the columns of the "Tested with" table, in order.
var testedNames = []struct{ key, name string }{
	{"go", "Go"},
	{"fabric", "Hyperledger Fabric"},
	{"fabric_x_committer", "Fabric-x committer"},
	{"fabric_x_tools", "Fabric-x tools"},
	{"postgres", "PostgreSQL"},
}

// notes returns the body of the GitHub release of d: a link to the upgrade guide, the versions
// the release was tested with, a link to the release data, and the generated changelog unchanged.
// Both files are linked rather than quoted, so the release body keeps no second copy of them. The
// guide is linked on main, where a fix to it reaches every reader of the release; the data file
// is linked at the tag, since a released record never changes.
// The links point to repository repo, owner/name, which a release from a fork sets to the
// fork.
func notes(dir, repo string, d Data, changelog string) (string, error) {
	if _, ok := parseVersion(d.Version); !ok {
		return "", errors.Errorf("invalid version [%s]", d.Version)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(d.Upgrade.Guide))); err != nil {
		return "", errors.Wrapf(err, "a minor needs its upgrade guide %s", d.Upgrade.Guide)
	}

	blobURL := "https://github.com/" + repo + "/blob/"
	var b strings.Builder
	fmt.Fprintf(&b, "## Upgrading\n\nRead the [upgrade guide](%smain/%s) before upgrading. What changed is listed below.\n\n",
		blobURL, d.Upgrade.Guide)
	b.WriteString("## Tested with\n\n")
	var names, seps, versions []string
	for _, t := range testedNames {
		names = append(names, t.name)
		seps = append(seps, "---")
		versions = append(versions, d.TestedWith[t.key])
	}
	fmt.Fprintf(&b, "| %s |\n|%s|\n| %s |\n\n", strings.Join(names, " | "), strings.Join(seps, "|"), strings.Join(versions, " | "))
	fmt.Fprintf(&b, "Every store's schema steps and the component version ranges are in the [release data](%s%s/%s/%s.json).\n\n",
		blobURL, d.Version, releasesDir, d.Version)
	b.WriteString(strings.TrimSpace(changelog) + "\n")
	return b.String(), nil
}
