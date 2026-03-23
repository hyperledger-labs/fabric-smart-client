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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
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
		p.Container().Provide(grpc.NewClientProvider, dig.As(new(ledger.GRPCClientProvider))),
		p.Container().Provide(ledger.NewProvider),
		p.Container().Provide(finality.NewListenerManagerProvider),
		p.Container().Provide(digutils.Identity[*finality.Provider](), dig.As(new(finality.ListenerManagerProvider))),
		p.Container().Provide(queryservice.NewProvider, dig.As(new(queryservice.Provider))),
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
		digutils.Register[*ledger.Provider](p.Container()),
	)
}

func (p *SDK) Start(ctx context.Context) error {
	if !p.FabricEnabled() {
		return p.SDK.Start(ctx)
	}

	// Wire the finality Listener Manager Provider and Ledger Provider with the application's root context.
	// This context is cancelled when the FSC application shuts down.
	err := p.Container().Invoke(func(in struct {
		dig.In
		FinalityProvider *finality.Provider
		LedgerProvider   *ledger.Provider
	}) error {
		in.FinalityProvider.Initialize(ctx)
		in.LedgerProvider.Initialize(ctx)
		return nil
	})
	if err != nil {
		return err
	}

	return p.SDK.Start(ctx)
}
