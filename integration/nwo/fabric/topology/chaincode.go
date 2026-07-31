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
	// Image is the CCaaS container image reference. When set, the CCaaS deployer
	// uses it directly. In-tree helpers set it to the conventional fsc-cc/<name>:latest.
	Image string
	// Deploy selects the deployer: "" or "ccaas" (default) or "legacy".
	Deploy string
	// Extension is an optional per-chaincode ccaas.ChaincodeExtension, stored as
	// any to avoid a topology->ccaas import cycle. The CCaaS deployer type-asserts it.
	Extension any `yaml:"-"`
}

// Conventional CCaaS image names for the in-tree test chaincodes. The Makefile
// CHAINCODE_IMAGES list must mirror these exactly.
const (
	ImageBaseChaincode    = "fsc-cc/base:latest"
	ImageStateChaincode   = "fsc-cc/state-query:latest"
	ImageEventsChaincode  = "fsc-cc/events:latest"
	ImageEvents2Chaincode = "fsc-cc/events2:latest"
	ImageATSAChaincode    = "fsc-cc/atsachaincode:latest"
)

// pathToImage maps in-tree chaincode Go import paths to their conventional
// CCaaS image name. External chaincodes use namespace.WithImage instead.
var pathToImage = map[string]string{
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/chaincode/base":      ImageBaseChaincode,
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/cc/query":    ImageStateChaincode,
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode":        ImageEventsChaincode,
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode2":       ImageEvents2Chaincode,
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsachaincode/chaincode": ImageATSAChaincode,
}

// ImageForPath returns the conventional CCaaS image name for a known in-tree
// chaincode Go import path, or "" if the path is not one of them.
func ImageForPath(p string) string {
	return pathToImage[p]
}

func (c *Chaincode) SetPackageIDFromPackageFile() {
	fileBytes, err := os.ReadFile(c.PackageFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	hashStr := fmt.Sprintf("%x", sha256.Sum256(fileBytes))
	c.PackageID = c.Label + ":" + hashStr
}

type PrivateChaincode struct {
	Image           string
	SGXMode         string
	SGXDevicesPaths []string
	MREnclave       string
}

type namespace struct {
	cc *ChannelChaincode
}

func (n *namespace) SetStateChaincode() *namespace {
	n.cc.Chaincode.Path = "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/cc/query"
	n.cc.Chaincode.Image = ImageStateChaincode
	return n
}

func (n *namespace) SetChaincodePath(path string) *namespace {
	n.cc.Chaincode.Path = path
	n.cc.Chaincode.Image = pathToImage[path]
	return n
}

func (n *namespace) NoInit() *namespace {
	n.cc.Chaincode.InitRequired = false
	return n
}

func (n *namespace) WithImage(ref string) *namespace {
	n.cc.Chaincode.Image = ref
	return n
}

func (n *namespace) AsLegacy() *namespace {
	n.cc.Chaincode.Deploy = "legacy"
	return n
}

func (n *namespace) WithChaincodeExtension(ext any) *namespace {
	n.cc.Chaincode.Extension = ext
	return n
}
