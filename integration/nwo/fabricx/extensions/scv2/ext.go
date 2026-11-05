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

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	fabric_topo "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
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

// Extension contributes the Fabric-x committers of a network: the mock orderer, in a
// container of its own, and one container per committer, each with its own sidecar identity,
// database and ports, all reading the same blocks from that orderer.
type Extension struct {
	channel *fabric_topo.Channel
	network *network.Network
	// committers holds every committer, the network's default first.
	committers []*committer
	// ordererEnvVars overrides the env of the mock orderer's container.
	ordererEnvVars map[string]string
}

// committer is one committer of the network, see Extension.
type committer struct {
	ext *Extension
	cfg CommitterConfig
}

// NewExtension returns the committer extension for the given network, with cfgs as its
// committers and ordererEnvVars overriding the mock orderer's env. The first committer is the
// network's default: FSC nodes of an organization without a committer of its own use it, and
// nwo reads the channel configuration from it.
func NewExtension(topology *fabric_topo.Topology, network *network.Network, ordererEnvVars map[string]string, cfgs ...CommitterConfig) *Extension {
	if len(topology.Channels) != 1 {
		panic(fmt.Sprintf("expected exactly one channel, got %d", len(topology.Channels)))
	}
	if len(cfgs) == 0 {
		panic("expected at least one committer")
	}
	e := &Extension{
		channel:        topology.Channels[0],
		network:        network,
		ordererEnvVars: ordererEnvVars,
	}
	names := map[string]bool{}
	for _, cfg := range cfgs {
		if len(network.PeersInOrg(cfg.SidecarOrg)) == 0 {
			panic(fmt.Sprintf("org [%s] does not exist in the network", cfg.SidecarOrg))
		}
		if names[cfg.SidecarName] {
			panic(fmt.Sprintf("committer name [%s] is used twice", cfg.SidecarName))
		}
		names[cfg.SidecarName] = true
		e.committers = append(e.committers, &committer{ext: e, cfg: cfg})
	}
	return e
}

// CheckTopology registers each committer's sidecar peer and its ports on the topology.
func (e *Extension) CheckTopology() {
	for _, c := range e.committers {
		// we add "another peer" for the committer to use its credentials
		c.addSCPeer()
	}
}

// GenerateArtifacts writes the query and notification service configuration into each FSC
// node's core.yaml.
func (e *Extension) GenerateArtifacts() {
	// generate fsc node core yaml extensions
	generateQSExtension(e)
	generateNSExtension(e)
}

// PostRun starts the mock orderer and every committer once the network is up.
func (e *Extension) PostRun(_ bool) {
	e.launchContainers()
}

// committersOf returns the committers of org, or the first committer when org has none.
func (e *Extension) committersOf(org string) []*committer {
	var cs []*committer
	for _, c := range e.committers {
		if c.cfg.SidecarOrg == org {
			cs = append(cs, c)
		}
	}
	if len(cs) == 0 {
		return e.committers[:1]
	}
	return cs
}

func (c *committer) addSCPeer() {
	e := c.ext
	// resolve ports: use pre-allocated ones or reserve new ones
	ports := c.cfg.SidecarPorts
	if len(ports) == 0 {
		ports = api.Ports{
			fabric_network.ListenPort:    e.network.Context.ReservePort(), // sidecar and notification service
			network.QueryServicePortName: e.network.Context.ReservePort(), // query service
		}
	}

	// we add "another peer" for the committer to use its credentials
	logger.Infof("Add committer [%s] for org=%v", c.cfg.SidecarName, c.cfg.SidecarOrg)
	p := &fabric_topo.Peer{
		Name:         c.cfg.SidecarName,
		Organization: c.cfg.SidecarOrg,
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
	if len(c.cfg.SidecarHost) > 0 {
		e.network.Context.SetHostByPeerID(e.network.Prefix, p.ID(), c.cfg.SidecarHost)
	}
}

// endpointTemplate carries the CONFIGURATION of one endpoint, which is what an extension
// writes into a node's core.yaml.
//
// Deliberately not grpc.ConnectionConfig: that type holds RESOLVED TLS — bytes already read
// from disk — whereas a template has to emit the paths the node will resolve for itself.
type endpointTemplate struct {
	Address           string
	ConnectionTimeout time.Duration
	TLSEnabled        bool
	RootCertPaths     []string
	ClientCertPath    string
	ClientKeyPath     string
}

// generateExtensions writes t into the core.yaml of every FSC node of n, with one endpoint
// per committer of the node's organization, see Extension.committersOf; addr returns a
// committer's address.
func generateExtensions(e *Extension, addr func(*committer) string, t *template.Template) {
	n := e.network
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
			continue // node has no identity in this Fabric network
		}

		// When TLS is enabled, the sidecar query service requires mTLS.
		// Use the fabric peer's TLS certs (signed by the fabric org CA)
		// as client credentials so the sidecar accepts the connection.
		var endpoints []endpointTemplate
		for _, c := range e.committersOf(fscPeer.Organization) {
			endpoint := endpointTemplate{
				Address:           addr(c),
				ConnectionTimeout: grpc.DefaultConnectionTimeout,
			}
			if n.TLSEnabled {
				tlsDir := n.PeerLocalTLSDir(fscPeer)
				endpoint.TLSEnabled = true
				endpoint.ClientCertPath = filepath.Join(tlsDir, "server.crt")
				endpoint.ClientKeyPath = filepath.Join(tlsDir, "server.key")
				endpoint.RootCertPaths = []string{n.CACertsBundlePath()}
			}
			endpoints = append(endpoints, endpoint)
		}

		data := extensionData{
			NetworkName:    topoName,
			RequestTimeout: 10 * time.Second,
			Endpoints:      endpoints,
		}

		var extension bytes.Buffer
		gomega.Expect(t.Execute(&extension, data)).NotTo(gomega.HaveOccurred())

		for _, uniqueName := range fscNode.ReplicaUniqueNames() {
			context.AddExtension(uniqueName, api.FabricExtension, extension.String())
		}
	}
}
