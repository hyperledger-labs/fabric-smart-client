/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"bytes"
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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger()

// CommitterConfig holds all configuration for the committer test container and its
// sidecar peer identity in the Fabric network.
type CommitterConfig struct {
	SidecarName  string
	SidecarOrg   string
	SidecarHost  string
	SidecarPorts api.Ports
	Image        string
	EnvVars      map[string]string
}

// Extension contributes the Fabric-x sidecar services to a network topology.
type Extension struct {
	channel *fabric_topo.Channel
	network *network.Network
	cfg     CommitterConfig
}

// NewExtension returns the sidecar extension for the given network.
func NewExtension(topology *fabric_topo.Topology, network *network.Network, cfg CommitterConfig) *Extension {
	if len(topology.Channels) != 1 {
		panic(fmt.Sprintf("expected exactly one channel, got %d", len(topology.Channels)))
	}
	if len(network.PeersInOrg(cfg.SidecarOrg)) == 0 {
		panic(fmt.Sprintf("org [%s] does not exist in the network", cfg.SidecarOrg))
	}
	return &Extension{
		channel: topology.Channels[0],
		network: network,
		cfg:     cfg,
	}
}

// CheckTopology registers the sidecar peer and its ports on the topology.
func (e *Extension) CheckTopology() {
	// we add "another peer" for the committer to use its credentials
	e.addSCPeer()
}

// GenerateArtifacts writes the query and notification service configuration into each FSC
// node's core.yaml.
func (e *Extension) GenerateArtifacts() {
	// generate fsc node core yaml extensions
	generateQSExtension(e.network, e.cfg.SidecarOrg, e.cfg.SidecarName)
	generateNSExtension(e.network, e.cfg.SidecarOrg, e.cfg.SidecarName)
}

// PostRun starts the sidecar container once the network is up.
func (e *Extension) PostRun(load bool) {
	// TODO: we want to launch one SC per org
	// run the docker container
	e.launchContainer()
}

func (e *Extension) addSCPeer() {
	// resolve ports: use pre-allocated ones or reserve new ones
	ports := e.cfg.SidecarPorts
	if len(ports) == 0 {
		ports = api.Ports{
			fabric_network.ListenPort:    e.network.Context.ReservePort(), // sidecar and notification service
			network.QueryServicePortName: e.network.Context.ReservePort(), // query service
		}
	}

	// we add "another peer" for the committer to use its credentials
	logger.Infof("Add committer for org=%v", e.cfg.SidecarOrg)
	p := &fabric_topo.Peer{
		Name:         e.cfg.SidecarName,
		Organization: e.cfg.SidecarOrg,
		Type:         fabric_topo.FabricPeer,
		Bootstrap:    false,
		Channels:     []*fabric_topo.PeerChannel{{Name: e.channel.Name, Anchor: false}},
		Usage:        "",
		SkipInit:     true,
		SkipRunning:  true,
		TLSDisabled:  !e.network.TLSEnabled,
	}
	e.network.AppendPeer(p)
	e.network.Context.SetPortsByPeerID(e.network.Prefix, p.ID(), ports)
	if len(e.cfg.SidecarHost) > 0 {
		e.network.Context.SetHostByPeerID(e.network.Prefix, p.ID(), e.cfg.SidecarHost)
	}
}

// endpointTemplate carries the CONFIGURATION of one endpoint, which is what an extension
// writes into a node's core.yaml.
//
// Deliberately not config.Endpoint: that type now holds RESOLVED TLS — bytes already read
// from disk — whereas a template has to emit the paths the node will resolve for itself.
type endpointTemplate struct {
	Address           string
	ConnectionTimeout time.Duration
	TLSEnabled        bool
	RootCertPaths     []string
	ClientCertPath    string
	ClientKeyPath     string
}

func generateExtensions(n *network.Network, scAddr string, t *template.Template) {
	context := n.Context

	fscTopo, ok := context.TopologyByName("fsc").(*fsc.Topology)
	if !ok {
		panic(fmt.Sprintf("expected *fsc.Topology, got %T", context.TopologyByName("fsc")))
	}

	type extensionData struct {
		NetworkName    string
		RequestTimeout time.Duration
		Endpoints      []endpointTemplate
	}

	topoName := n.Topology().Name()
	for _, fscNode := range fscTopo.Nodes {
		// check that the fsc node is associated with the fabric network
		fscPeer := n.FSCPeerByName(fscNode.Name)
		if fscPeer == nil {
			continue // node has no identity in this Fabric network (e.g. pure bootstrap node)
		}

		// When TLS is enabled, the sidecar query service requires mTLS.
		// Use the fabric peer's TLS certs (signed by the fabric org CA)
		// as client credentials so the sidecar accepts the connection.
		endpoint := endpointTemplate{
			Address:           scAddr,
			ConnectionTimeout: grpc.DefaultConnectionTimeout,
		}
		if n.TLSEnabled {
			tlsDir := n.PeerLocalTLSDir(fscPeer)
			endpoint.TLSEnabled = true
			endpoint.ClientCertPath = filepath.Join(tlsDir, "server.crt")
			endpoint.ClientKeyPath = filepath.Join(tlsDir, "server.key")
			endpoint.RootCertPaths = []string{n.CACertsBundlePath()}
		}

		data := extensionData{
			NetworkName:    topoName,
			RequestTimeout: 10 * time.Second,
			Endpoints:      []endpointTemplate{endpoint},
		}

		var extension bytes.Buffer
		utils.Must(t.Execute(&extension, data))

		for _, uniqueName := range fscNode.ReplicaUniqueNames() {
			context.AddExtension(uniqueName, api.FabricExtension, extension.String())
		}
	}
}
