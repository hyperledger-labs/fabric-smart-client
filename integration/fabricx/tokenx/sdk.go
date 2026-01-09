/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tokenx

import (
	"errors"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK is the Token Management SDK that extends the FabricX SDK
type SDK struct {
	common.SDK
}

// NewSDK creates a new Token Management SDK from a services registry
func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabricx.NewSDK(registry))
}

// NewFrom creates a Token Management SDK from an existing SDK
func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

// Install registers all required services for the token management application
func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
	)
}
