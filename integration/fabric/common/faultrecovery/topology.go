/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package faultrecovery holds what the Fabric and Fabric-x faultrecovery suites share:
// the FSC nodes, their views, and the helpers the specs drive them with.
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
// installing sdk on both. Each node runs on its own Postgres database, named
// dbPrefix-alice and dbPrefix-bob.
//
// A node receives only the data source of its database. nwo starts each container
// from that data source (postgres.ConfigFromDataSource), which names the container
// after the database, so StopPostgres(dbPrefix+"-bob") finds bob's container. For the
// same reason a container name set on the ContainerConfig here would not reach the
// container. Suites that may run at the same time need different prefixes.
func FSCTopology(sdk node.SDK, dbPrefix string) *fsc.Topology {
	db := &integration.ReplicationOptions{SQLConfigs: map[string]*postgres.ContainerConfig{
		"alice": postgres.DefaultConfig(postgres.WithDBName(dbPrefix + "-alice")),
		"bob":   postgres.DefaultConfig(postgres.WithDBName(dbPrefix + "-bob")),
	}}

	fscTopology := fsc.NewTopology()
	fscTopology.EnablePrometheusMetrics()
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(db.For("alice")...).
		RegisterViewFactory("submit", &views.SubmitViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(db.For("bob")...).
		RegisterResponder(&views.ResponderView{}, &views.SubmitView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})
	fscTopology.AddSDK(sdk)

	return fscTopology
}
