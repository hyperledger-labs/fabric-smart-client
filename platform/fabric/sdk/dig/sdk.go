/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/config"
	finality2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"go.uber.org/dig"
)

var logger = flogging.MustGetLogger("fabric-sdk")

type SDK struct {
	dig2.SDK
	fnsProvider *core.FSNProvider
}

func NewSDK(registry node.Registry) *SDK {
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
		p.Container().Provide(config.NewCore),
		p.Container().Provide(config.NewProvider),
		p.Container().Provide(NewChannelProvider),
		p.Container().Provide(NewDriver, dig.Group("drivers")),
		p.Container().Provide(finality2.NewHandler, dig.Group("finality-handlers")),
		p.Container().Provide(identity.NewProvider),
		p.Container().Provide(NewFSNProvider),
		p.Container().Provide(digutils.Identity[*core.FSNProvider](), dig.As(new(driver.FabricNetworkServiceProvider))),
		p.Container().Provide(digutils.Identity[*endpoint.Service](), dig.As(new(identity.EndpointService))),
		p.Container().Provide(fabric.NewNetworkServiceProvider),
		p.Container().Provide(vault.NewService, dig.As(new(state.VaultService))),
		p.Container().Provide(NewEndorserTransactionHandlerProvider),
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
	logger.Debugf("SDK installation complete:\n%s", digutils.Visualize(p.Container()))

	if err := p.Container().Invoke(registerFinalityHandlers); err != nil {
		return err
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

func registerFinalityHandlers(in struct {
	dig.In
	FinalityManager *finality.Manager
	Handlers        []finality.Handler `group:"finality-handlers"`
}) {
	for _, handler := range in.Handlers {
		in.FinalityManager.AddHandler(handler)
	}
}

func registerProcessorsForDrivers(in struct {
	dig.In
	CoreConfig             *core.Config
	NetworkServiceProvider *fabric.NetworkServiceProvider
	Drivers                []core.NamedDriver `group:"drivers"`
}) error {
	if len(in.CoreConfig.Names()) == 0 {
		return fmt.Errorf("no fabric network names found")
	}

	for _, d := range in.Drivers {
		logger.Infof("trying to install for driver: %s", d.Name)
		if c, err := in.CoreConfig.Config(in.CoreConfig.DefaultName()); err != nil || c.Driver != d.Name {
			logger.Infof("Skipping registration of default network, because its driver is %s. We are registering %s", c.Driver, d.Name)
			return nil
		}
		defaultFns, err := in.NetworkServiceProvider.FabricNetworkService("")
		if err != nil {
			return fmt.Errorf("could not find default FNS: %w", err)
		}
		for _, name := range in.CoreConfig.Names() {
			if c, err := in.CoreConfig.Config(name); err != nil || c.Driver != d.Name {
				logger.Infof("Skipping registration because network driver [%s] is not the selected driver [%s]", c.Driver, d)
				continue
			} else {
				logger.Infof("did not skip: %s", c.Driver)
			}
			fns, err := in.NetworkServiceProvider.FabricNetworkService(name)
			if err != nil {
				return fmt.Errorf("could not find FNS [%s]: %w", name, err)
			}
			if err := fns.ProcessorManager().SetDefaultProcessor(state.NewRWSetProcessor(defaultFns)); err != nil {
				return fmt.Errorf("failed setting state processor for fabric network [%s]: %w", name, err)
			}
		}
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
		return fmt.Errorf("failed starting fabric network service provider: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := p.fnsProvider.Stop(); err != nil {
			logger.Errorf("failed stopping fabric network service provider [%s]", err)
		}
	}()

	return nil
}

func registerRWSetLoaderHandlerProviders(in struct {
	dig.In
	FSNProvider      *core.FSNProvider
	CoreConfig       *core.Config
	HandlerProviders []RWSetPayloadHandlerProvider `group:"handler-providers"`
}) error {
	for _, network := range in.CoreConfig.Names() {
		fsn, err := in.FSNProvider.FabricNetworkService(network)
		if err != nil {
			return fmt.Errorf("could not find network service for %s: %w", network, err)
		}
		for _, channelName := range fsn.ConfigService().ChannelIDs() {
			ch, err := fsn.Channel(channelName)
			if err != nil {
				return fmt.Errorf("could not find channel %s for network %s: %w", channelName, network, err)
			}
			loader := ch.RWSetLoader()
			for _, handlerProvider := range in.HandlerProviders {
				if err := loader.AddHandlerProvider(handlerProvider.Type, handlerProvider.New); err != nil {
					return fmt.Errorf("failed to add handler to channel %s: %w", channelName, err)
				}
			}
		}
	}
	return nil
}
