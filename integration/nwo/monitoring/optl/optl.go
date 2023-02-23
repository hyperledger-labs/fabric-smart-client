/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	. "github.com/onsi/gomega"
)

var logger = flogging.MustGetLogger("integration.nwo.monitoring.optl")

type Platform interface {
	GetContext() api.Context
	ConfigDir() string
	NetworkID() string
	OPTL() bool
	OPTLPort() int
}

type Extension struct {
	platform Platform
}

func NewExtension(platform Platform) *Extension {
	return &Extension{
		platform: platform,
	}
}

func (n *Extension) CheckTopology() {
	if !n.platform.OPTL() {
		return
	}
}

func (n *Extension) GenerateArtifacts() {
	if !n.platform.OPTL() {
		return
	}

	Expect(os.MkdirAll(n.configFileDir(), 0o777)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.configFilePath(), []byte(ConfigTemplate), 0o644)).NotTo(HaveOccurred())
}

func (n *Extension) PostRun(bool) {
	if !n.platform.OPTL() {
		return
	}

	// start HL Explorer as docker containers
	n.startContainer()
}

func (n *Extension) configFileDir() string {
	return filepath.Join(
		n.platform.ConfigDir(),
		"optl",
	)
}

func (n *Extension) configFilePath() string {
	return filepath.Join(n.configFileDir(), "optl-collector-config.yaml")
}
