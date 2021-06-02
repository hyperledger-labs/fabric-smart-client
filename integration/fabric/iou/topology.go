/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []nwo.Topology {
	// Create an empty fabric topology
	fabricTopology := fabric.NewDefaultTopology()
	// Add two organizations with one peer each
	fabricTopology.AddOrganizationsByMapping(map[string][]string{
		"Org1": {"alice"},
		"Org2": {"bob"},
	})
	// Deploy a dummy chaincode to setup the namespace
	fabricTopology.AddNamespace(
		"iou",
		`AND ('Org1MSP.member','Org2MSP.member')`,
		"alice", "bob",
	)

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()

	// Add the initiator fsc node
	borrower := fscTopology.AddNodeByName("borrower")
	borrower.AddOptions(fabric.WithOrganization("Org1"))
	borrower.RegisterViewFactory("create", &CreateIOUInitiatorViewFactory{})
	borrower.RegisterViewFactory("update", &UpdateIOUInitiatorViewFactory{})
	borrower.RegisterViewFactory("query", &QueryViewFactory{})

	// Add the responder fsc node
	lender := fscTopology.AddNodeByName("lender")
	lender.AddOptions(fabric.WithOrganization("Org2"))
	lender.RegisterResponder(&CreateIOUResponderView{}, &CreateIOUInitiatorView{})
	lender.RegisterResponder(&UpdateIOUResponderView{}, &UpdateIOUInitiatorView{})
	lender.RegisterViewFactory("query", &QueryViewFactory{})

	return []nwo.Topology{fabricTopology, fscTopology}
}
