/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"go.uber.org/dig"
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
		// Register the new fabricx platform driver
		p.Container().Provide(NewDriver, dig.Group("fabric-platform-drivers")),
		p.Container().Provide(NewChannelProvider, dig.As(new(ChannelProvider))),
		p.Container().Provide(ledger.NewEventBasedProvider, dig.As(new(ledger.Provider))),
		p.Container().Provide(ledger.NewBlockDispatcherProvider),
		p.Container().Provide(finality.NewListenerManagerProvider),
		p.Container().Provide(digutils.Identity[*finality.Provider](), dig.As(new(finality.ListenerManagerProvider))),
		p.Container().Provide(queryservice.NewProvider, dig.As(new(queryservice.Provider))),
		p.Container().Provide(fabricx.NewNetworkServiceProvider),
	)
	if err != nil {
		return err
	}

	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Backward compatibility with SP
	return errors.Join(
		digutils.Register[finality.ListenerManagerProvider](p.Container()),
		digutils.Register[queryservice.Provider](p.Container()),
	)
}

func (p *SDK) Start(ctx context.Context) error {
	if !p.FabricEnabled() {
		return p.SDK.Start(ctx)
	}

	// Wire the finality Listener Manager Provider with the application's root context.
	// This context is cancelled when the FSC application shuts down.
	// By initializing the provider with this context, we ensure that during shutdown,
	// all active finality listener instances (which run in goroutines managed by the provider)
	// are properly notified and can terminate their long-running streams.
	err := p.Container().Invoke(func(in struct {
		dig.In
		Provider *finality.Provider
	}) error {
		in.Provider.Initialize(ctx)
		return nil
	})
	if err != nil {
		return err
	}

	return p.SDK.Start(ctx)
}
