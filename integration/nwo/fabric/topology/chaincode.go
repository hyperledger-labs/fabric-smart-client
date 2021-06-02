/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package topology

import (
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/common/util"
	"github.com/onsi/gomega"
)

type Chaincode struct {
	Name                string
	Version             string
	Path                string
	Ctor                string
	Policy              string // only used for legacy lifecycle. For new lifecycle use SignaturePolicy
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
}

func (c *Chaincode) SetPackageIDFromPackageFile() {
	fileBytes, err := ioutil.ReadFile(c.PackageFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	hashStr := fmt.Sprintf("%x", util.ComputeSHA256(fileBytes))
	c.PackageID = c.Label + ":" + hashStr
}
