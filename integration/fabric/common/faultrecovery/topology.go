/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package faultrecovery holds what the Fabric and Fabric-x faultrecovery suites share:
// the FSC nodes, their views, the helpers that drive them, and the specs both suites run.
package faultrecovery

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	configviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/configupdate/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// FSCTopology returns the FSC nodes alice (Org1, initiator), bob and charlie (Org2,
// responders), installing sdk on all three. Each serves configseq, the sequence of the
// channel configuration the node holds, and ledger, which reads a transaction back through
// the ledger API and supports Fabric only. Alice and bob each run on their own Postgres
// database, named by DBName; charlie has none, so nwo gives him SQLite, the default
// persistence, and the crash specs cover a node on it.
//
// A node receives only the data source of its database. nwo starts each container
// from that data source (postgres.ConfigFromDataSource), which names the container
// after the database, so StopPostgres(DBName(dbPrefix, "bob")) finds bob's container. For the
// same reason a container name set on the ContainerConfig here would not reach the
// container. Suites that may run at the same time need different prefixes.
func FSCTopology(sdk node.SDK, dbPrefix string) *fsc.Topology {
	return newFSCTopology(sdk, dbPrefix, nil, nil)
}

// ReplicatedFSCTopology returns the nodes of FSCTopology with two replicas each of alice
// and bob, every replica on its node's one database; charlie has a single replica. Bob's
// replicas sit behind a load balancer (fsc.WithLoadBalancer); alice has none. Alice
// multiplexes all her sessions to bob over one websocket, which the load balancer hands to
// one of his replicas, so that replica serves every session until the connection closes
// and alice dials again, when the round robin may pick the other one.
func ReplicatedFSCTopology(sdk node.SDK, dbPrefix string) *fsc.Topology {
	return newFSCTopology(sdk, dbPrefix,
		map[string]int{"alice": 2, "bob": 2},
		map[string][]fscnode.Option{"bob": {fsc.WithLoadBalancer()}},
	)
}

func newFSCTopology(sdk node.SDK, dbPrefix string, replicas map[string]int, opts map[string][]fscnode.Option) *fsc.Topology {
	db := &integration.ReplicationOptions{
		ReplicationFactors: replicas,
		SQLConfigs: map[string]*postgres.ContainerConfig{
			"alice": postgres.DefaultConfig(postgres.WithDBName(DBName(dbPrefix, "alice"))),
			"bob":   postgres.DefaultConfig(postgres.WithDBName(DBName(dbPrefix, "bob"))),
		},
		NodeOptions: opts,
	}

	fscTopology := fsc.NewTopology()
	fscTopology.EnablePrometheusMetrics()
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(db.For("alice")...).
		RegisterViewFactory("endorse", &views.EndorseViewFactory{}).
		RegisterViewFactory("endorseLiar", &views.LiarEndorseViewFactory{}).
		RegisterViewFactory("forgedTxID", &views.ForgedTxIDViewFactory{}).
		RegisterViewFactory("honestExchange", &views.HonestExchangeViewFactory{}).
		RegisterViewFactory("forgedRecipient", &views.ForgedRecipientViewFactory{}).
		RegisterViewFactory("order", &views.OrderViewFactory{}).
		RegisterViewFactory("broadcast", &views.BroadcastViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{}).
		RegisterViewFactory("ledger", &cviews.LedgerViewFactory{}).
		RegisterViewFactory("echo", &views.EchoViewFactory{}).
		RegisterViewFactory("configseq", &configviews.ConfigSequenceViewFactory{})
	for _, name := range []string{"bob", "charlie"} {
		fscTopology.AddNodeByName(name).
			AddOptions(fabric.WithOrganization("Org2")).
			AddOptions(db.For(name)...).
			RegisterResponder(&views.ResponderView{}, &views.EndorseView{}).
			RegisterResponder(&views.LiarResponderView{}, &views.LiarEndorseView{}).
			RegisterResponder(&views.ResponderView{}, &views.ForgedTxIDView{}).
			RegisterResponder(&views.EchoResponderView{}, &views.EchoView{}).
			RegisterResponder(&views.RecipientResponderView{}, &views.HonestExchangeView{}).
			RegisterResponder(&views.RecipientResponderView{}, &views.ForgedRecipientView{}).
			RegisterViewFactory("recipientOwner", &views.RecipientOwnerViewFactory{}).
			RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
			RegisterViewFactory("query", &views.QueryViewFactory{}).
			RegisterViewFactory("ledger", &cviews.LedgerViewFactory{}).
			RegisterViewFactory("configseq", &configviews.ConfigSequenceViewFactory{})
	}
	fscTopology.AddSDK(sdk)

	return fscTopology
}

// DBName returns the name of node's Postgres database in an FSCTopology built with dbPrefix.
func DBName(dbPrefix, node string) string { return dbPrefix + "-" + node }
