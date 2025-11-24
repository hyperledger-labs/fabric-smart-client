/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otlp

import (
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/onsi/gomega"
)

var logger = logging.MustGetLogger()

type Platform interface {
	GetContext() api.Context
	ConfigDir() string
	NetworkID() string
	OTLP() bool
	OTLPPort() int
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
	if !n.platform.OTLP() {
		return
	}
}

func (n *Extension) GenerateArtifacts() {
	if !n.platform.OTLP() {
		return
	}

	gomega.Expect(os.MkdirAll(n.configFileDir(), 0o777)).NotTo(gomega.HaveOccurred())
}

func (n *Extension) PostRun(bool) {
	if !n.platform.OTLP() {
		return
	}

	// start HL Explorer as docker containers
	n.startContainer()
}

func (n *Extension) configFileDir() string {
	return filepath.Join(
		n.platform.ConfigDir(),
		"otlp",
	)
}
