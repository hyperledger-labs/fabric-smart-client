/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"fmt"
	"maps"
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
	return numberedCommitterName(org, 0)
}

// numberedCommitterName is the name of org's i-th committer.
func numberedCommitterName(org string, i int) string {
	return fmt.Sprintf("%s_committer_%d", org, i)
}

// CommitterConfig holds all configuration for the committer test container and
// its corresponding sidecar peer entry in the Fabric network.
// EnvVars is merged over the container defaults: a non-empty value overrides or
// adds a var; an empty string value removes the default.
type CommitterConfig struct {
	// Fabric network identity
	Name string
	Org  string

	// Optional extra host name in the sidecar's TLS certificate; FSC nodes reach the committer at 127.0.0.1
	Host string

	// Optional pre-allocated ports (nil = auto-allocate)
	Ports api.Ports

	// Container overrides
	Image   string
	EnvVars map[string]string
}

// CommitterOption configures a committer added with AddCommitter.
type CommitterOption func(*CommitterConfig)

// WithName names the committer, used for its sidecar identity; by default it is the next
// free <org>_committer_<n>.
func WithName(name string) CommitterOption {
	return func(c *CommitterConfig) { c.Name = name }
}

// WithHost adds host to the TLS certificate of the committer's sidecar.
func WithHost(host string) CommitterOption {
	return func(c *CommitterConfig) { c.Host = host }
}

// WithPorts sets pre-allocated ports for the committer's container.
func WithPorts(ports api.Ports) CommitterOption {
	return func(c *CommitterConfig) { c.Ports = ports }
}

// WithImage sets the Docker image of the committer's container, over the topology's.
func WithImage(image string) CommitterOption {
	return func(c *CommitterConfig) { c.Image = image }
}

// WithEnv sets or overrides an env var of the committer's container, over the topology's.
// Pass an empty value to remove a default var.
func WithEnv(key, value string) CommitterOption {
	return func(c *CommitterConfig) {
		if c.EnvVars == nil {
			c.EnvVars = map[string]string{}
		}
		c.EnvVars[key] = value
	}
}

// Topology extends the fabric topology with fabricx-specific committer options.
//
// Every organization with a peer runs a committer, named CommitterName(org), the way each
// Fabric organization runs <org>_peer_0. AddCommitter adds more, such as a second one in an
// organization. CommitterImage and CommitterEnvVars apply to every committer, unless one added
// with AddCommitter overrides them. Every committer reads the same blocks from the one mock
// orderer, and each FSC node uses the committers of its own organization. OrdererEnvVars
// configures the mock orderer.
type Topology struct {
	*fabric_topology.Topology
	CommitterImage   string
	CommitterEnvVars map[string]string
	Committers       []CommitterConfig
	OrdererEnvVars   map[string]string
}

// AddCommitter adds a committer for the organization org, with its own container, database
// and ports, reading the same blocks as the other committers, configured by opts.
func (t *Topology) AddCommitter(org string, opts ...CommitterOption) *Topology {
	c := CommitterConfig{Org: org}
	for _, opt := range opts {
		opt(&c)
	}
	t.Committers = append(t.Committers, c)
	return t
}

// AllCommitters returns the committer of each organization with a peer, then those
// AddCommitter added. The first is the network's default committer, the first organization's.
// A committer added without a name gets the next free <org>_committer_<n>; every committer
// takes CommitterImage and CommitterEnvVars unless it sets its own image or env var.
func (t *Topology) AllCommitters() []CommitterConfig {
	var all []CommitterConfig
	used := map[string]bool{}
	for _, org := range t.peerOrgs() {
		all = append(all, CommitterConfig{Name: CommitterName(org), Org: org})
		used[CommitterName(org)] = true
	}
	for _, c := range t.Committers {
		if c.Name == "" {
			for i := 0; ; i++ {
				if c.Name = numberedCommitterName(c.Org, i); !used[c.Name] {
					break
				}
			}
		}
		used[c.Name] = true
		all = append(all, c)
	}
	for i := range all {
		if all[i].Image == "" {
			all[i].Image = t.CommitterImage
		}
		env := maps.Clone(t.CommitterEnvVars)
		if env == nil && all[i].EnvVars != nil {
			env = map[string]string{}
		}
		maps.Copy(env, all[i].EnvVars)
		all[i].EnvVars = env
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

// WithCommitterImage sets the Docker image of every committer's container, and of the mock
// orderer's.
func (t *Topology) WithCommitterImage(image string) *Topology {
	t.CommitterImage = image
	return t
}

// WithCommitterEnv sets or overrides an env var of every committer's container.
// Pass an empty value to remove a default var.
func (t *Topology) WithCommitterEnv(key, value string) *Topology {
	if t.CommitterEnvVars == nil {
		t.CommitterEnvVars = map[string]string{}
	}
	t.CommitterEnvVars[key] = value
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
