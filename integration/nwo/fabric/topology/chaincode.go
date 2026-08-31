/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/onsi/gomega"
)

type Chaincode struct {
	Name                string
	Version             string
	Path                string
	Ctor                string
	Lang                string
	CollectionsConfig   string // optional
	PackageFile         string
	PackageID           string            `yaml:"packageID,omitempty"` // if unspecified, chaincode won't be executable. Can use SetPackageIDFromPackageFile() to set.
	CodeFiles           map[string]string // map from paths on the filesystem to code.tar.gz paths
	Sequence            string
	EndorsementPlugin   string
	ValidationPlugin    string
	InitRequired        bool
	Label               string
	SignaturePolicy     string
	ChannelConfigPolicy string
	// Image is the CCaaS container image reference. When set, nwo runs one
	// chaincode server container per organization from this image. When unset,
	// Path names Go source that the peer packages and builds itself.
	Image string
}

// IsCCaaS reports whether this chaincode deploys as a Chaincode-as-a-Service
// container. Image and Path are mutually exclusive; AddNamespace enforces it.
func (c *Chaincode) IsCCaaS() bool { return c.Image != "" }

func (c *Chaincode) SetPackageIDFromPackageFile() {
	fileBytes, err := os.ReadFile(c.PackageFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	hashStr := fmt.Sprintf("%x", sha256.Sum256(fileBytes))
	c.PackageID = c.Label + ":" + hashStr
}
