/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"go.uber.org/dig"
)

var logger = logging.MustGetLogger()

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
		p.Container().Provide(queryservice.NewProvider, dig.As(new(queryservice.Provider))),
	)
	if err != nil {
		return err
	}

	err = errors.Join(
		p.Container().Decorate(NewConfigProvider),
		p.Container().Decorate(NewHostProvider),
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
	return p.SDK.Start(ctx)
}
