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
	preamble.RegisterComponent(pingpong.TestComponent, 2, 3)
	if err := n.InstallSDK(libp2psupport.NewFrom(viewsdk.NewSDK(n))); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	cli.Execute(n, func() error {
		registry := view.GetRegistry(n)
		if err := registry.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("mockInit", &fake.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("stream", &pingpong.StreamerViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("unhosted", &pingpong.UnhostedViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("version", &pingpong.VersionInitiatorViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
