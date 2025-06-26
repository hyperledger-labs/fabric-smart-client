/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/mock"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(sdk.NewSDK(node)); err != nil {
		panic(err)
	}
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		initiatorID := registry.GetIdentifier(&pingpong.Initiator{})
		if err := registry.RegisterResponder(&pingpong.Responder{}, initiatorID); err != nil {
			return err
		}
		if err := registry.RegisterResponder(&pingpong.Responder{}, &mock.Initiator{}); err != nil {
			return err
		}
		return nil
	})
}
