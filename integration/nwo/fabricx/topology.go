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
	// DefaultCommitterName is the name of the first committer unless WithCommitterName sets one.
	DefaultCommitterName = "SC"
)

// CommitterName is the name EnableCommitterPerOrg gives org's committer, after the
// <org>_peer_0 naming of Fabric peers.
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
// Committer is the network's first committer, the one every topology has; the WithCommitter*
// builders set it. With EnableCommitterPerOrg every other organization with a peer gets a
// committer of its own too, the way each Fabric organization gets a peer. Committers lists
// further committers, added with AddCommitter, such as a second one in an organization. Every
// committer reads the same blocks from the one mock orderer, and each FSC node uses the
// committers of its own organization, or the first committer when its organization has none.
type Topology struct {
	*fabric_topology.Topology
	Committer       CommitterConfig
	Committers      []CommitterConfig
	CommitterPerOrg bool
}

// EnableCommitterPerOrg gives every organization with a peer a committer, named
// CommitterName(org), and the first committer, unless WithCommitterName named it, too.
func (t *Topology) EnableCommitterPerOrg() *Topology {
	t.CommitterPerOrg = true
	return t
}

// AddCommitter adds a committer named name for the organization org, with its own
// container, database and ports, reading the same blocks as the first committer.
func (t *Topology) AddCommitter(org, name string) *Topology {
	t.Committers = append(t.Committers, CommitterConfig{Name: name, Org: org})
	return t
}

// AllCommitters returns the first committer, then, with EnableCommitterPerOrg, one for each
// other organization with a peer, then those AddCommitter added. A committer other than the
// first takes the first committer's image and env vars unless it has its own.
func (t *Topology) AllCommitters() []CommitterConfig {
	first := t.Committer
	var rest []CommitterConfig
	if t.CommitterPerOrg {
		if first.Name == DefaultCommitterName {
			first.Name = CommitterName(first.Org)
		}
		for _, org := range t.peerOrgs() {
			if org != first.Org {
				rest = append(rest, CommitterConfig{Name: CommitterName(org), Org: org})
			}
		}
	}
	all := []CommitterConfig{first}
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

// WithCommitterName sets the peer name used for the committer sidecar identity.
func (t *Topology) WithCommitterName(name string) *Topology {
	t.Committer.Name = name
	return t
}

// WithCommitterOrg sets the organization the committer belongs to.
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

// WithCommitterEnv sets or overrides a single committer container env var.
// Pass an empty value to remove a default var.
func (t *Topology) WithCommitterEnv(key, value string) *Topology {
	if t.Committer.EnvVars == nil {
		t.Committer.EnvVars = map[string]string{}
	}
	t.Committer.EnvVars[key] = value
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

	// the ordering service provided by the committer all-in-one supports TLS
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
		Committer: CommitterConfig{
			Name: DefaultCommitterName,
			Org:  "Org1",
		},
	}
}
