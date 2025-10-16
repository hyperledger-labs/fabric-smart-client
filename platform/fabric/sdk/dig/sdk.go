/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig/fns"
	generic2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig/generic"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/vault"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"go.uber.org/dig"
)

var logger = logging.MustGetLogger()

type SDK struct {
	dig2.SDK
	fnsProvider *core.FSNProvider
}

func NewSDK(registry services.Registry) *SDK {
	return NewFrom(viewsdk.NewSDK(registry))
}

func NewFrom(sdk dig2.SDK) *SDK {
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
		p.Container().Provide(config.NewProvider),
		p.Container().Provide(committer.NewFinalityListenerManagerProvider[driver.ValidationCode], dig.As(new(driver.ListenerManagerProvider))),
		p.Container().Provide(generic2.NewDriver, dig.Group("fabric-platform-drivers")),
		p.Container().Provide(generic2.NewChannelProvider, dig.Name("generic-channel-provider")),
		p.Container().Provide(fns.NewProvider),
		p.Container().Provide(digutils.Identity[*core.FSNProvider](), dig.As(new(driver.FabricNetworkServiceProvider))),
		p.Container().Provide(digutils.Identity[*endpoint.Service](), dig.As(new(identity.EndpointService))),
		p.Container().Provide(digutils.Identity[*id.Provider](), dig.As(new(identity.ViewIdentityProvider))),
		p.Container().Provide(fabric.NewNetworkServiceProvider),
		p.Container().Provide(vault.NewService, dig.As(new(state.VaultService))),
		p.Container().Provide(generic2.NewEndorserTransactionHandlerProvider),

		p.Container().Provide(committer2.NewSerialDependencyResolver, dig.As(new(committer2.DependencyResolver))),
		p.Container().Provide(postgres.NewNamedDriver, dig.Group("fabric-db-drivers")),
		p.Container().Provide(sqlite.NewNamedDriver, dig.Group("fabric-db-drivers")),
		p.Container().Provide(mem.NewNamedDriver, dig.Group("fabric-db-drivers")),
		p.Container().Provide(generic2.NewMultiplexedDriver),
		p.Container().Provide(generic2.NewMetadataStore),
		p.Container().Provide(generic2.NewEnvelopeStore),
		p.Container().Provide(generic2.NewEndorseTxStore),
	)
	if err != nil {
		return err
	}

	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Backward compatibility with SP
	return errors.Join(
		digutils.Register[*fabric.NetworkServiceProvider](p.Container()), // GetFabricNetworkService is used by many components
	)
}

func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	if !p.FabricEnabled() {
		return nil
	}

	if err := p.Container().Invoke(registerProcessorsForDrivers); err != nil {
		return err
	}
	if err := p.Container().Invoke(registerRWSetLoaderHandlerProviders); err != nil {
		return err
	}
	if err := p.Container().Invoke(func(fnsProvider *core.FSNProvider) { p.fnsProvider = fnsProvider }); err != nil {
		return err
	}
	return nil
}

func (p *SDK) PostStart(ctx context.Context) error {
	if err := p.SDK.PostStart(ctx); err != nil {
		return err
	}

	if !p.FabricEnabled() {
		logger.Infof("Fabric platform not enabled, skipping start")
		return p.SDK.PostStart(ctx)
	}
	if p.fnsProvider == nil {
		return errors.New("no fabric network provider found")
	}

	if err := p.fnsProvider.Start(ctx); err != nil {
		return errors.Wrapf(err, "failed starting fabric network service provider")
	}

	go func() {
		<-ctx.Done()
		if err := p.fnsProvider.Stop(); err != nil {
			logger.Errorf("failed stopping fabric network service provider [%s]", err)
		}
	}()

	return nil
}

func registerProcessorsForDrivers(in struct {
	dig.In
	NetworkServiceProvider *fabric.NetworkServiceProvider
}) error {
	for _, name := range in.NetworkServiceProvider.Names() {
		fns, err := in.NetworkServiceProvider.FabricNetworkService(name)
		if err != nil {
			return errors.Wrapf(err, "failed to get fabric network [%s]", name)
		}
		if err := fns.ProcessorManager().SetDefaultProcessor(state.NewRWSetProcessor(fns)); err != nil {
			return errors.Wrapf(err, "failed setting state processor for fabric network [%s]", name)
		}
	}
	return nil
}

func registerRWSetLoaderHandlerProviders(in struct {
	dig.In
	FSNProvider      *core.FSNProvider
	HandlerProviders []generic2.RWSetPayloadHandlerProvider `group:"handler-providers"`
}) error {
	for _, network := range in.FSNProvider.Names() {
		fsn, err := in.FSNProvider.FabricNetworkService(network)
		if err != nil {
			return errors.Wrapf(err, "could not find network service for %s", network)
		}
		for _, channelName := range fsn.ConfigService().ChannelIDs() {
			ch, err := fsn.Channel(channelName)
			if err != nil {
				return errors.Wrapf(err, "could not find channel %s for network %s", channelName, network)
			}
			loader := ch.RWSetLoader()
			for _, handlerProvider := range in.HandlerProviders {
				if err := loader.AddHandlerProvider(handlerProvider.Type, handlerProvider.New); err != nil {
					return errors.Wrapf(err, "failed to add handler to channel %s", channelName)
				}
			}
		}
	}
	return nil
}
