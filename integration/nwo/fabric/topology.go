/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/opts"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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

func WithOrganization(organization string) node.Option {
	return func(o *node.Options) error {
		Options(o).AddOrganization(organization)
		return nil
	}
}

func WithNetworkOrganization(network, organization string) node.Option {
	return func(o *node.Options) error {
		Options(o).AddNetworkOrganization(network, organization)
		return nil
	}
}

func WithDefaultNetwork(network string) node.Option {
	return func(o *node.Options) error {
		Options(o).SetDefaultNetwork(network)
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

// WithDefaultPostgresPersistence makes config the database of the node's default persistence,
// which holds every store that names no other persistence. A nil config leaves the node as is.
func WithDefaultPostgresPersistence(config DataSourceProvider) node.Option {
	return WithPostgresPersistence(common.DefaultPersistence, config)
}

// WithPostgresPersistence defines the Postgres persistence name with config as its database. nwo
// starts that database with the network. A nil config leaves the node as is.
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

// WithStorePersistence routes the store identified by key, such as "fsc_kvs" for fsc.kvs, to
// the persistence name, which another option must define.
func WithStorePersistence(key node.PersistenceKey, name driver.PersistenceName) node.Option {
	return func(o *node.Options) error {
		o.PutPostgresPersistenceName(key, name)
		return nil
	}
}

// WithTablePrefix sets the table prefix of the Postgres persistence name, which an earlier
// option must define.
func WithTablePrefix(name driver.PersistenceName, prefix string) node.Option {
	return updatePostgresPersistence(name, func(opts *node.SQLOpts) { opts.TablePrefix = prefix })
}

// WithPostgresTLS makes nwo start the database of the Postgres persistence name, which an
// earlier option must define, with TLS, and the node connect to it with ssl_mode verify-full.
// Every persistence that shares the database is then served over TLS.
func WithPostgresTLS(name driver.PersistenceName) node.Option {
	return updatePostgresPersistence(name, func(opts *node.SQLOpts) { opts.TLS = true })
}

func updatePostgresPersistence(name driver.PersistenceName, update func(*node.SQLOpts)) node.Option {
	return func(o *node.Options) error {
		opts := o.GetPostgresPersistence(name)
		if opts == nil {
			return errors.Errorf("postgres persistence [%s] is not defined", name)
		}
		update(opts)
		o.PutPostgresPersistence(name, *opts)
		return nil
	}
}

// WithDefaultSqlitePersistence gives the node a SQLite default persistence for its stores, and
// a SQLite file of its own for the vault of its Fabric networks, which Platform.DeleteVault
// removes.
func WithDefaultSqlitePersistence() node.Option {
	return func(o *node.Options) error {
		o.PutPersistenceKey(node.PersistenceKey(common.DefaultPersistence))
		o.PutPersistenceKey(network.VaultPersistenceKey)
		return nil
	}
}

// Network returns the fabric network from the passed context bound to the passed id.
// It returns nil, if nothing is found
func Network(ctx *context.NWOContext, id string) *Platform {
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
