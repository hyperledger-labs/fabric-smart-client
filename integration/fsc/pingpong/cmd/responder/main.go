/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/mock"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/registry"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
	node := fscnode.NewEmpty("")
	utils.Must(node.InstallSDK(viewsdk.NewSDK(node)))
	node.Execute(func() error {
		initiatorID := registry2.GetIdentifier(&pingpong.Initiator{})
		if err := node.RegisterResponder(&pingpong.Responder{}, initiatorID); err != nil {
			return err
		}
		if err := node.RegisterResponder(&pingpong.Responder{}, &mock.Initiator{}); err != nil {
			return err
		}
		return nil
	})
}
