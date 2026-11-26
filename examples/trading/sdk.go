/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package trading

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

// SDK is the fabricx SDK plus the services the trading views look up: the
// state vault and the node's local key-value store, where a seller keeps the
// private terms of its listings.
type SDK struct {
	common.SDK
}

// NewSDK returns the trading SDK on top of a fresh fabricx SDK.
func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabricx.NewSDK(registry))
}

// NewFrom returns the trading SDK on top of sdk.
func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

// Install installs the wrapped SDK, then registers the trading services.
func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
		digutils.Register[*kvs.KVS](p.Container()),
	)
}
