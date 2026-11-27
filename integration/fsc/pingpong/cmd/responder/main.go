/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/fake"
	libp2psupport "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/support/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/node/cli"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	n := node.NewFromConfPath("")
	preamble.RegisterComponent(pingpong.TestComponent, 1, 2)
	if err := n.InstallSDK(libp2psupport.NewFrom(viewsdk.NewSDK(n))); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	cli.Execute(n, func() error {
		registry := view.GetRegistry(n)
		initiatorID := registry.GetIdentifier(&pingpong.Initiator{})
		if err := registry.RegisterResponderWithOptions(&pingpong.Responder{}, initiatorID, view.WithProtocol("pingpong", "ping")); err != nil {
			return err
		}
		if err := registry.RegisterResponder(&pingpong.VersionResponder{}, &pingpong.VersionInitiator{}); err != nil {
			return err
		}
		if err := registry.RegisterResponder(&pingpong.Responder{}, &fake.Initiator{}); err != nil {
			return err
		}
		return nil
	})
}
