/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_topology "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

const (
	TopologyName        = PlatformName
	DefaultTopologyName = "default"
)

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
type Topology struct {
	*fabric_topology.Topology
	Committer CommitterConfig
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

	return &Topology{
		Topology: topo,
		Committer: CommitterConfig{
			Name: "SC",
			Org:  "Org1",
		},
	}
}
