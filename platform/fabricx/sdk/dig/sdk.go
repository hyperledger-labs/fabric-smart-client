/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK extends the fabric SDK with fabricX.
type SDK struct {
	common.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabric.NewSDK(registry))
}

func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

func (p *SDK) FabricEnabled() bool {
	return p.ConfigService().GetBool("fabric.enabled")
}

func (p *SDK) Install() error {
	if !p.FabricEnabled() {
		return p.SDK.Install()
	}
	err := errors.Join(
		endorser.Install(p.SDK),
		client.Install(p.SDK),
	)
	if err != nil {
		return err
	}

	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Backward compatibility with SP
	return client.RegisterLegacy(p.SDK)
}

func (p *SDK) Start(ctx context.Context) error {
	if !p.FabricEnabled() {
		return p.SDK.Start(ctx)
	}

	// Wire the client-side providers with the application's root context.
	if err := client.InitializeProviders(p.SDK, ctx); err != nil {
		return err
	}

	return p.SDK.Start(ctx)
}
