/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

import (
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/onsi/gomega"
)

var logger = logging.MustGetLogger("integration.nwo.monitoring.optl")

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

	gomega.Expect(os.MkdirAll(n.configFileDir(), 0o777)).NotTo(gomega.HaveOccurred())
	gomega.Expect(os.WriteFile(n.configFilePath(), []byte(ConfigTemplate), 0o644)).NotTo(gomega.HaveOccurred())
	gomega.Expect(os.WriteFile(n.jaegerHostsPath(), []byte(JaegerHosts), 0o644)).NotTo(gomega.HaveOccurred())
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

func (n *Extension) jaegerHostsPath() string {
	return filepath.Join(n.configFileDir(), "jaeger_hosts")
}
