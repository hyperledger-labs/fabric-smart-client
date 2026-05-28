/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"text/template"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	fabric_topo "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
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
	// generate fsc node core yaml extensions
	generateQSExtension(e.network, e.sidecar.Org, e.sidecar.Name)
	generateNSExtension(e.network, e.sidecar.Org, e.sidecar.Name)
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
		e.sidecar.Ports[fabric_network.ListenPort] = e.network.Context.ReservePort()    // sidecar and notification service
		e.sidecar.Ports[network.QueryServicePortName] = e.network.Context.ReservePort() // query service
	}

	for _, org := range e.network.PeerOrgs() {
		logger.Infof("Add committer for org=%v", org.Name)
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
			TLSDisabled:  !e.network.TLSEnabled,
		}
		e.network.AppendPeer(p)
		e.network.Context.SetPortsByPeerID(e.network.Prefix, p.ID(), e.sidecar.Ports)
		if len(e.sidecar.Host) > 0 {
			e.network.Context.SetHostByPeerID(e.network.Prefix, p.ID(), e.sidecar.Host)
		}
	}
}

func generateExtensions(n *network.Network, scAddr string, t *template.Template) {
	context := n.Context

	fscTop, ok := context.TopologyByName("fsc").(*fsc.Topology)
	if !ok {
		utils.Must(errors.New("cannot get fsc topo instance"))
	}

	type extensionData struct {
		NetworkName    string
		RequestTimeout time.Duration
		Endpoints      []config.Endpoint
	}

	topoName := n.Topology().Name()
	for _, fscNode := range fscTop.Nodes {
		// When TLS is enabled, the sidecar query service requires mTLS.
		// Use the fabric peer's TLS certs (signed by the fabric org CA)
		// as client credentials so the sidecar accepts the connection.
		var tlsConfig config.TLSConfig
		if n.TLSEnabled {
			fscPeer := n.FSCPeerByName(fscNode.Name)
			if fscPeer == nil {
				utils.Must(fmt.Errorf("failed to get fsc peer by name: %v", fscNode.Name))
			}
			tlsDir := n.PeerLocalTLSDir(fscPeer)
			tlsConfig.Enabled = true
			tlsConfig.ClientCertPath = filepath.Join(tlsDir, "server.crt")
			tlsConfig.ClientKeyPath = filepath.Join(tlsDir, "server.key")
			tlsConfig.RootCertPaths = []string{n.CACertsBundlePath()}
		}

		endpoint := config.Endpoint{
			Address:           scAddr,
			ConnectionTimeout: grpc.DefaultConnectionTimeout,
			TLS:               &tlsConfig,
		}

		data := extensionData{
			NetworkName:    topoName,
			RequestTimeout: 10 * time.Second,
			Endpoints:      []config.Endpoint{endpoint},
		}

		var extension bytes.Buffer
		err := t.Execute(&extension, data)
		utils.Must(err)

		for _, uniqueName := range fscNode.ReplicaUniqueNames() {
			context.AddExtension(uniqueName, api.FabricExtension, extension.String())
		}
	}
}
