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

// notes returns the body of the GitHub release of d: how to upgrade, the versions the release
// was tested with, a link to the release data, and the generated changelog unchanged. A minor
// links its upgrade guide, which must exist in dir; a patch has no guide and says so. Both files
// are linked rather than quoted, so the release body keeps no second copy of them. The guide is
// linked on main, where a fix to it reaches every reader of the release; the data file is linked
// at the tag, since a released record never changes.
// The links point to repository repo, owner/name, which a release from a fork sets to the
// fork.
func notes(dir, repo string, d Data, changelog string) (string, error) {
	v, ok := parseVersion(d.Version)
	if !ok {
		return "", errors.Errorf("invalid version [%s]", d.Version)
	}
	blobURL := "https://github.com/" + repo + "/blob/"
	var b strings.Builder
	switch {
	case v.Patch > 0:
		fmt.Fprintf(&b, "## Upgrading\n\nNo breaking changes. Upgrade from v%d.%d.x at any time, one node at a time. What changed is listed below.\n\n",
			v.Major, v.Minor)
	case d.Upgrade.Guide == "":
		return "", errors.Errorf("%s is a minor and has no upgrade guide", v)
	default:
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(d.Upgrade.Guide))); err != nil {
			return "", errors.Wrapf(err, "a minor needs its upgrade guide %s", d.Upgrade.Guide)
		}
		fmt.Fprintf(&b, "## Upgrading\n\nRead the [upgrade guide](%smain/%s) before upgrading. What changed is listed below.\n\n",
			blobURL, d.Upgrade.Guide)
	}
	b.WriteString("## Tested with\n\n")
	var names, seps, versions []string
	for _, t := range tested {
		names = append(names, t.heading)
		seps = append(seps, "---")
		versions = append(versions, d.TestedWith[t.key])
	}
	fmt.Fprintf(&b, "| %s |\n|%s|\n| %s |\n\n", strings.Join(names, " | "), strings.Join(seps, "|"), strings.Join(versions, " | "))
	fmt.Fprintf(&b, "Every store's schema steps and the component version ranges are in the [release data](%s%s/%s/%s.json).\n\n",
		blobURL, d.Version, releasesDir, d.Version)
	b.WriteString(strings.TrimSpace(changelog) + "\n")
	return b.String(), nil
}
