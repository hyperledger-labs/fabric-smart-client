/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auction

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

// SDK is the fabricx SDK plus the services the auction views look up: the
// state vault and the node's local key-value store, where the house keeps the
// bids it endorsed and each bidder keeps its own bid.
type SDK struct {
	common.SDK
}

// NewSDK returns the auction SDK on top of a fresh fabricx SDK.
func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabricx.NewSDK(registry))
}

// NewFrom returns the auction SDK on top of sdk.
func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

// Install installs the wrapped SDK, then registers the auction's services.
func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
		digutils.Register[*kvs.KVS](p.Container()),
	)
}
