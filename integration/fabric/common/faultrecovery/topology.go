/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package faultrecovery holds what the Fabric and Fabric-x faultrecovery suites share:
// the FSC nodes, their views, the helpers that drive them, and the specs both suites run.
package faultrecovery

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// FSCTopology returns the FSC nodes alice (Org1, initiator) and bob (Org2, responder),
// installing sdk on both. Each node runs on its own Postgres database, named by DBName.
//
// A node receives only the data source of its database. nwo starts each container
// from that data source (postgres.ConfigFromDataSource), which names the container
// after the database, so StopPostgres(DBName(dbPrefix, "bob")) finds bob's container. For the
// same reason a container name set on the ContainerConfig here would not reach the
// container. Suites that may run at the same time need different prefixes.
func FSCTopology(sdk node.SDK, dbPrefix string) *fsc.Topology {
	db := &integration.ReplicationOptions{SQLConfigs: map[string]*postgres.ContainerConfig{
		"alice": postgres.DefaultConfig(postgres.WithDBName(DBName(dbPrefix, "alice"))),
		"bob":   postgres.DefaultConfig(postgres.WithDBName(DBName(dbPrefix, "bob"))),
	}}

	fscTopology := fsc.NewTopology()
	fscTopology.EnablePrometheusMetrics()
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(db.For("alice")...).
		RegisterViewFactory("endorse", &views.EndorseViewFactory{}).
		RegisterViewFactory("order", &views.OrderViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(db.For("bob")...).
		RegisterResponder(&views.ResponderView{}, &views.EndorseView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})
	fscTopology.AddSDK(sdk)

	return fscTopology
}

// DBName returns the name of node's Postgres database in an FSCTopology built with dbPrefix.
func DBName(dbPrefix, node string) string { return dbPrefix + "-" + node }
