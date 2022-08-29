/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/views"
)

func main() {
	n := fscnode.New()
	n.InstallSDK(fabric.NewSDK(n))
	n.Execute(func() error {
		registry := viewregistry.GetRegistry(n)
		if err := registry.RegisterFactory("create", &views.CreateIOUViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("update", &views.UpdateIOUViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterFactory("query", &views.QueryViewFactory{}); err != nil {
			return err
		}

		return nil
	})
}
