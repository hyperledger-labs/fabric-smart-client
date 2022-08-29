/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	iou "github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/views"
)

func main() {
	n := fscnode.New()
	n.InstallSDK(fabric.NewSDK(n))
	n.Execute(func() error {
		registry := viewregistry.GetRegistry(n)
		registry.RegisterResponder(&iou.ApproverView{}, &iou.CreateIOUView{})
		registry.RegisterResponder(&iou.ApproverView{}, &iou.UpdateIOUView{})

		return nil
	})
}
