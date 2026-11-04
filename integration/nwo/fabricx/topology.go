/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_topology "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

const (
	TopologyName        = PlatformName
	DefaultTopologyName = "default"
)

// CommitterName is the name of org's committer, after the <org>_peer_0 naming of Fabric peers.
func CommitterName(org string) string {
	return org + "_committer_0"
}

// CommitterConfig holds all configuration for the committer test container and
// its corresponding sidecar peer entry in the Fabric network.
// EnvVars is merged over the container defaults: a non-empty value overrides or
// adds a var; an empty string value removes the default.
type CommitterConfig struct {
	// Fabric network identity
	Name string
	Org  string

	// Optional fixed host for the sidecar peer (empty = use docker bridge IP)
	Host string

	// Optional pre-allocated ports (nil = auto-allocate)
	Ports api.Ports

	// Container overrides
	Image   string
	EnvVars map[string]string
}

// Topology extends the fabric topology with fabricx-specific committer options.
//
// Every organization with a peer runs a committer, named CommitterName(org), the way each
// Fabric organization runs <org>_peer_0. Committer configures the network's first committer,
// that of the first organization unless WithCommitterOrg picks another; the WithCommitter*
// builders set it, and the other committers take its image and env vars unless they set their
// own. Committers lists further committers, added with AddCommitter, such as a second one in an
// organization. Every committer reads the same blocks from the one mock orderer, and each FSC
// node uses the committers of its own organization. OrdererEnvVars configures the mock orderer.
type Topology struct {
	*fabric_topology.Topology
	Committer      CommitterConfig
	Committers     []CommitterConfig
	OrdererEnvVars map[string]string
}

// AddCommitter adds a committer named name for the organization org, with its own
// container, database and ports, reading the same blocks as the other committers.
func (t *Topology) AddCommitter(org, name string) *Topology {
	t.Committers = append(t.Committers, CommitterConfig{Name: name, Org: org})
	return t
}

// AllCommitters returns the first committer, then the committer of each other organization
// with a peer, then those AddCommitter added. The first committer belongs to Committer.Org, or
// to the first organization with a peer when Committer.Org is empty, and is named
// Committer.Name, or CommitterName(org) when that is empty.
func (t *Topology) AllCommitters() []CommitterConfig {
	orgs := t.peerOrgs()
	first := t.Committer
	if first.Org == "" && len(orgs) > 0 {
		first.Org = orgs[0]
	}
	if first.Name == "" {
		first.Name = CommitterName(first.Org)
	}
	all := []CommitterConfig{first}
	var rest []CommitterConfig
	for _, org := range orgs {
		if org != first.Org {
			rest = append(rest, CommitterConfig{Name: CommitterName(org), Org: org})
		}
	}
	for _, c := range append(rest, t.Committers...) {
		if c.Image == "" {
			c.Image = first.Image
		}
		if c.EnvVars == nil {
			c.EnvVars = first.EnvVars
		}
		all = append(all, c)
	}
	return all
}

// peerOrgs returns the organizations with at least one peer, in the order they were added.
func (t *Topology) peerOrgs() []string {
	var orgs []string
	for _, o := range t.Organizations {
		if slices.ContainsFunc(t.Peers, func(p *fabric_topology.Peer) bool { return p.Organization == o.Name }) {
			orgs = append(orgs, o.Name)
		}
	}
	return orgs
}

// WithCommitterName sets the name of the first committer, used for its sidecar identity.
func (t *Topology) WithCommitterName(name string) *Topology {
	t.Committer.Name = name
	return t
}

// WithCommitterOrg sets the organization of the first committer.
func (t *Topology) WithCommitterOrg(org string) *Topology {
	t.Committer.Org = org
	return t
}

// WithCommitterHost sets a fixed host for the sidecar peer entry.
func (t *Topology) WithCommitterHost(host string) *Topology {
	t.Committer.Host = host
	return t
}

// WithCommitterPorts sets pre-allocated ports for the committer container.
func (t *Topology) WithCommitterPorts(ports api.Ports) *Topology {
	t.Committer.Ports = ports
	return t
}

// WithCommitterImage sets the Docker image for the committer test container.
func (t *Topology) WithCommitterImage(image string) *Topology {
	t.Committer.Image = image
	return t
}

// WithCommitterEnv sets or overrides a single committer container env var, for the first
// committer and every committer that sets no env vars of its own.
// Pass an empty value to remove a default var.
func (t *Topology) WithCommitterEnv(key, value string) *Topology {
	if t.Committer.EnvVars == nil {
		t.Committer.EnvVars = map[string]string{}
	}
	t.Committer.EnvVars[key] = value
	return t
}

// WithOrdererEnv sets or overrides a single env var of the mock orderer container, such as
// SC_ORDERER_BLOCK_SIZE. Pass an empty value to remove a default var.
func (t *Topology) WithOrdererEnv(key, value string) *Topology {
	if t.OrdererEnvVars == nil {
		t.OrdererEnvVars = map[string]string{}
	}
	t.OrdererEnvVars[key] = value
	return t
}

// SetDefault marks this topology as the default network. Shadows the embedded
// method so the return type stays *Topology and fluent chaining is preserved.
func (t *Topology) SetDefault() *Topology {
	t.Topology.SetDefault()
	return t
}

// NewTopology returns a new topology whose name is empty.
func NewTopology() *Topology {
	return NewTopologyWithName("")
}

// NewDefaultTopology is a configuration with two organizations and one peer per org.
func NewDefaultTopology() *Topology {
	return NewTopologyWithName(DefaultTopologyName).SetDefault()
}

// NewTopologyWithName returns a fabricx topology with the given name.
func NewTopologyWithName(name string) *Topology {
	topo := fabric.NewTopologyWithName(name)

	topo.SetLogging("grpc=error:info", "")

	// the mock ordering service of the committer test image supports TLS
	topo.TLSEnabled = true
	topo.ClientAuthRequired = true

	// set fabricx specific settings
	topo.TopologyType = PlatformName
	topo.Driver = PlatformName

	topo.Orderers = []*fabric_topology.Orderer{
		{Name: "orderer1", Organization: "OrdererOrg", Id: 1},
		{Name: "orderer2", Organization: "OrdererOrg", Id: 2},
		{Name: "orderer3", Organization: "OrdererOrg", Id: 3},
		{Name: "orderer4", Organization: "OrdererOrg", Id: 4},
	}
	for _, p := range topo.Profiles {
		p.Orderers = []string{"orderer1", "orderer2", "orderer3", "orderer4"}
	}

	return &Topology{
		Topology: topo,
	}
}
