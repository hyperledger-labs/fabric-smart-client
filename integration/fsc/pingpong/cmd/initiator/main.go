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
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
	node := fscnode.NewEmpty("")
	utils.Must(node.InstallSDK(viewsdk.NewSDK(node)))
	node.Execute(func() error {
		if err := node.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := node.RegisterFactory("mockInit", &mock.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := node.RegisterFactory("stream", &pingpong.StreamerViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
