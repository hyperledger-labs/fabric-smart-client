/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/opts"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
)

var (
	ImplicitMetaReaders              = &topology.Policy{Name: "Readers", Type: "ImplicitMeta", Rule: "ANY Readers"}
	ImplicitMetaWriters              = &topology.Policy{Name: "Writers", Type: "ImplicitMeta", Rule: "ANY Writers"}
	ImplicitMetaAdmins               = &topology.Policy{Name: "Admins", Type: "ImplicitMeta", Rule: "ANY Admins"}
	ImplicitMetaLifecycleEndorsement = &topology.Policy{Name: "LifecycleEndorsement", Type: "ImplicitMeta", Rule: "MAJORITY Endorsement"}
	ImplicitMetaEndorsement          = &topology.Policy{Name: "Endorsement", Type: "ImplicitMeta", Rule: "ANY Endorsement"}
)

const (
	TopologyName = "fabric"
)

const (
	ClientRole = "client"
	PeerRole   = "peer"
)

func Options(o *node.Options) *opts.Options {
	return opts.Get(o)
}

func WithClientRole() node.Option {
	return func(o *node.Options) error {
		Options(o).SetRole(ClientRole)
		return nil
	}
}

func WithPeerRole() node.Option {
	return func(o *node.Options) error {
		Options(o).SetRole(PeerRole)
		return nil
	}
}

func WithOrganization(Organization string) node.Option {
	return func(o *node.Options) error {
		Options(o).AddOrganization(Organization)
		return nil
	}
}

func WithNetworkOrganization(Network, Organization string) node.Option {
	return func(o *node.Options) error {
		Options(o).AddNetworkOrganization(Network, Organization)
		return nil
	}
}

func WithDefaultNetwork(Network string) node.Option {
	return func(o *node.Options) error {
		Options(o).SetDefaultNetwork(Network)
		return nil
	}
}

// WithAnonymousIdentity adds support for anonymous identity
func WithAnonymousIdentity() node.Option {
	return func(o *node.Options) error {
		Options(o).SetAnonymousIdentity(true)
		return nil
	}
}

func WithX509Identity(label string) node.Option {
	return func(o *node.Options) error {
		fo := Options(o)
		fo.SetX509Identities(append(fo.X509Identities(), label))
		return nil
	}
}

func WithX509IdentityByHSM(label string) node.Option {
	return func(o *node.Options) error {
		fo := Options(o)
		fo.SetHSMX509Identities(append(fo.X509IdentitiesByHSM(), label))
		return nil
	}
}

func WithIdemixIdentity(label string) node.Option {
	return func(o *node.Options) error {
		fo := Options(o)
		fo.SetIdemixIdentities(append(fo.IdemixIdentities(), label))
		return nil
	}
}

// WithDefaultIdentityByHSM to make the default identity to be HSM identity
func WithDefaultIdentityByHSM() node.Option {
	return func(o *node.Options) error {
		Options(o).SetDefaultIdentityByHSM(true)
		return nil
	}
}

// WithDefaultIdentityWithLabel sets the label of the default identity
func WithDefaultIdentityWithLabel(label string) node.Option {
	return func(o *node.Options) error {
		Options(o).SetDefaultIdentityLabel(label)
		return nil
	}
}

// NewDefaultTopology is a configuration with two organizations and one peer per org.
func NewDefaultTopology() *topology.Topology {
	return NewTopologyWithName("default").SetDefault()
}

// NewTopology returns a new topology whose name is empty
func NewTopology() *topology.Topology {
	return NewTopologyWithName("")
}

// NewTopologyWithName is a configuration with two organizations and one peer per org
func NewTopologyWithName(name string) *topology.Topology {
	return &topology.Topology{
		TopologyName: name,
		TopologyType: "fabric",
		Driver:       "generic",
		TLSEnabled:   true,
		Logging: &topology.Logging{
			Spec:   "grpc=error:info",
			Format: "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'",
		},
		Organizations: []*topology.Organization{{
			Name:          "OrdererOrg",
			MSPID:         "OrdererMSP",
			MSPType:       "bccsp",
			Domain:        "example.com",
			EnableNodeOUs: false,
			Users:         0,
			CA:            &topology.CA{Hostname: "ca"},
		}},
		Consortiums: []*topology.Consortium{{
			Name: "SampleConsortium",
		}},
		Consensus: &topology.Consensus{
			Type: "etcdraft",
		},
		Orderers: []*topology.Orderer{
			{Name: "orderer", Organization: "OrdererOrg"},
		},
		Channels: []*topology.Channel{
			{Name: "testchannel", Profile: "OrgsChannel", Default: true},
		},
		Profiles: []*topology.Profile{{
			Name:            "OrgsChannel",
			Orderers:        []string{"orderer"},
			Consortium:      "SampleConsortium",
			AppCapabilities: []string{"V2_5"},
			Blocks: &topology.Blocks{
				BatchTimeout:      1,
				MaxMessageCount:   500,
				AbsoluteMaxBytes:  98,
				PreferredMaxBytes: 2,
			},
			Policies: []*topology.Policy{
				ImplicitMetaReaders,
				ImplicitMetaWriters,
				ImplicitMetaAdmins,
				ImplicitMetaLifecycleEndorsement,
				ImplicitMetaEndorsement,
			},
		}},
	}
}

type DataSourceProvider interface {
	DataSource() string
}

func WithDefaultPostgresPersistence(config DataSourceProvider) node.Option {
	return WithPostgresPersistence(common.DefaultPersistence, config)
}

func WithPostgresPersistence(name driver.PersistenceName, config DataSourceProvider) node.Option {
	return func(o *node.Options) error {
		if config != nil {
			o.PutPostgresPersistence(name, node.SQLOpts{
				DataSource: config.DataSource(),
			})
		}
		return nil
	}
}

func WithDefaultSqlitePersistence() node.Option {
	return func(o *node.Options) error {
		o.PutPersistenceKey(node.PersistenceKey(common.DefaultPersistence))
		return nil
	}
}

// Network returns the fabric network from the passed context bound to the passed id.
// It returns nil, if nothing is found
func Network(ctx *context.Context, id string) *Platform {
	p := ctx.PlatformByName(id)
	if p == nil {
		return nil
	}
	fp, ok := p.(*Platform)
	if ok {
		return fp
	}
	return nil
}
