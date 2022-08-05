/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hle

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	. "github.com/onsi/gomega"
)

var logger = flogging.MustGetLogger("integration.nwo.fabric.hle")

type Platform interface {
	HyperledgerExplorer() bool
	GetContext() api.Context
	ConfigDir() string
	NetworkID() string
	HyperledgerExplorerPort() int
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
	if !n.platform.HyperledgerExplorer() {
		return
	}
}

func (n *Extension) GenerateArtifacts() {
	if !n.platform.HyperledgerExplorer() {
		return
	}

	config := Config{
		NetworkConfigs: map[string]Network{},
		License:        "Apache-2.0",
	}

	// Generate and store config for each fabric network
	for _, platform := range n.platform.GetContext().PlatformsByType(fabric.TopologyName) {
		fabricPlatform := platform.(*fabric.Platform)

		networkName := fmt.Sprintf("hlf-%s", fabricPlatform.Topology().Name())
		config.NetworkConfigs[fabricPlatform.Topology().Name()] = Network{
			Name:                 fmt.Sprintf("Fabric Network (%s)", networkName),
			Profile:              "./connection-profile/" + fabricPlatform.Topology().Name() + ".json",
			EnableAuthentication: true,
		}

		// marshal config
		configJSON, err := json.MarshalIndent(config, "", "  ")
		Expect(err).NotTo(HaveOccurred())
		// write config to file
		Expect(os.MkdirAll(n.configFileDir(), 0o755)).NotTo(HaveOccurred())
		Expect(ioutil.WriteFile(n.configFilePath(), configJSON, 0o644)).NotTo(HaveOccurred())

		// Generate and store connection profile
		cp := fabricPlatform.ConnectionProfile(fabricPlatform.Topology().Name(), false)
		// add client section
		cp.Client = nnetwork.Client{
			AdminCredential: nnetwork.AdminCredential{
				Id:       "admin",
				Password: "admin",
			},
			Organization:         fabricPlatform.PeerOrgs()[0].Name,
			EnableAuthentication: true,
			TlsEnable:            true,
			Connection: nnetwork.Connection{
				Timeout: nnetwork.Timeout{
					Peer: map[string]string{
						"endorser": "600",
					},
				},
			},
		}
		cpJSON, err := json.MarshalIndent(cp, "", "  ")
		Expect(err).NotTo(HaveOccurred())
		// write cp to file
		Expect(os.MkdirAll(n.cpFileDir(), 0o755)).NotTo(HaveOccurred())
		Expect(ioutil.WriteFile(n.cpFilePath(fabricPlatform.Topology().Name()), cpJSON, 0o644)).NotTo(HaveOccurred())
	}
}

func (n *Extension) PostRun(bool) {
	if !n.platform.HyperledgerExplorer() {
		return
	}

	// start HL Explorer as docker containers
	n.startContainer()
}

func (n *Extension) configFileDir() string {
	return filepath.Join(
		n.platform.ConfigDir(),
		"hle",
	)
}

func (n *Extension) configFilePath() string {
	return filepath.Join(n.configFileDir(), "config.json")
}

func (n *Extension) cpFileDir() string {
	return filepath.Join(
		n.configFileDir(),
		"connection-profile",
	)
}

func (n *Extension) cpFilePath(name string) string {
	return filepath.Join(n.cpFileDir(), name+".json")
}
