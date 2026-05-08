/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincodetofsc

import (
	"errors"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK is the per-example FSC SDK. It composes the platform Fabric-X SDK
// (which brings in the Fabric-X driver, Query Service client, Arma client,
// and committer event plumbing) and additionally registers the state.VaultService
// so views that read state directly via vault.GetState work.
//
// Mirrors integration/fabricx/iou/sdk.go.
type SDK struct {
	common.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabricx.NewSDK(registry))
}

func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}
	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
	)
}
