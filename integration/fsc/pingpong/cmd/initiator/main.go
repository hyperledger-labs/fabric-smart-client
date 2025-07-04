/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/mock"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(sdk.NewSDK(node)); err != nil {
		panic(err)
	}
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		if err := registry.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("mockInit", &mock.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("stream", &pingpong.StreamerViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
