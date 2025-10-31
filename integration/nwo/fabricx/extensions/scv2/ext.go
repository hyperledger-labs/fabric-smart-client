/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	fabric_topo "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

var logger = logging.MustGetLogger()

type Extension struct {
	channel *fabric_topo.Channel
	network *network.Network

	sidecar *sidecarConfig
}

type sidecarConfig struct {
	Host  string
	Ports api.Ports
	Name  string
	Org   string
}

func NewExtension(topology *fabric_topo.Topology, network *network.Network, sidecarHost string, sidecarPorts api.Ports, sidecarName, sidecarOrg string) *Extension {
	if len(topology.Channels) != 1 {
		utils.Must(fmt.Errorf("we currently support a single channel"))
	}
	if len(network.PeersInOrg(sidecarOrg)) == 0 {
		panic(fmt.Sprintf("org [%s] does not exist in the network", sidecarOrg))
	}
	return &Extension{
		channel: topology.Channels[0],
		network: network,
		sidecar: &sidecarConfig{
			Host:  sidecarHost,
			Ports: sidecarPorts,
			Name:  sidecarName,
			Org:   sidecarOrg,
		},
	}
}

func (e *Extension) CheckTopology() {
	// we add "another peer" for the scalable committer to use its credentials
	e.addSCPeer()
}

func (e *Extension) GenerateArtifacts() {
	generateQSExtension(e.network)
	generateNSExtension(e.network)
}

func (e *Extension) PostRun(load bool) {
	// TODO: we want to launch one SC per org
	// run the docker container
	e.launchContainer()
}

func (e *Extension) addSCPeer() {
	if e.sidecar == nil {
		return
	}

	// reserve ports
	if len(e.sidecar.Ports) == 0 {
		e.sidecar.Ports = api.Ports{}
		for _, portName := range fabric_network.PeerPortNames() {
			e.sidecar.Ports[portName] = e.network.Context.ReservePort()
		}
	}

	for _, org := range e.network.PeerOrgs() {
		logger.Infof("Add (virtual) SC Peer for org=%v", org.Name)
		// we add "another peer" for the scalable committer to use its credentials
		p := &fabric_topo.Peer{
			Name:         e.sidecar.Name,
			Organization: org.Name,
			Type:         fabric_topo.FabricPeer,
			Bootstrap:    false,
			Channels:     []*fabric_topo.PeerChannel{{Name: e.channel.Name, Anchor: true}},
			Usage:        "",
			SkipInit:     true,
			SkipRunning:  true,
			TLSDisabled:  true,
			// Hostname:       "scalable-committer",
		}
		e.network.AppendPeer(p)
		e.network.Context.SetPortsByPeerID(e.network.Prefix, p.ID(), e.sidecar.Ports)
		if len(e.sidecar.Host) > 0 {
			e.network.Context.SetHostByPeerID(e.network.Prefix, p.ID(), e.sidecar.Host)
		}
	}
}
