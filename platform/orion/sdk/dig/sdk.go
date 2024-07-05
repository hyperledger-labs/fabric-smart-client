/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"context"
	"errors"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	finality2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"go.uber.org/dig"
)

var logger = flogging.MustGetLogger("orion-sdk")

type Registry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type Startable interface {
	Start(ctx context.Context) error
	Stop() error
}

type SDK struct {
	dig2.SDK
	onsProvider Startable
}

func NewSDK(registry node.Registry) *SDK {
	return NewFrom(viewsdk.NewSDK(registry))
}

func NewFrom(sdk dig2.SDK) *SDK {
	return &SDK{SDK: sdk}
}

func (p *SDK) OrionEnabled() bool {
	return p.ConfigService().GetBool("orion.enabled")
}

func (p *SDK) Install() error {
	if !p.OrionEnabled() {
		logger.Infof("Orion platform not enabled, skipping")
		return p.SDK.Install()
	}

	logger.Infof("Orion platform enabled, installing...")
	err := errors.Join(
		p.Container().Provide(digutils.Identity[driver.ConfigService](), dig.As(new(core.ConfigProvider))),
		p.Container().Provide(core.NewConfig),
		p.Container().Provide(newOrionNetworkServiceProvider),
		p.Container().Provide(orion.NewNetworkServiceProvider),
		p.Container().Provide(digutils.Identity[*core.ONSProvider](), dig.As(new(driver2.OrionNetworkServiceProvider))),
		p.Container().Provide(finality2.NewHandler, dig.Group("finality-handlers")),
	)
	if err != nil {
		return err
	}

	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Backward compatibility with SP
	return errors.Join(
		digutils.Register[*orion.NetworkServiceProvider](p.Container()),
		digutils.Register[*core.ONSProvider](p.Container()),
	)
}

func (p *SDK) Start(ctx context.Context) error {
	defer logger.Infof("SDK installation complete:\n%s", digutils.Visualize(p.Container()))
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}

	if !p.OrionEnabled() {
		logger.Infof("Orion platform not enabled, skipping start")
		return nil
	}
	logger.Infof("Orion platform enabled, starting...")

	return errors.Join(
		p.Container().Invoke(registerFinalityHandlers),
		p.Container().Invoke(func(onsProvider *core.ONSProvider) { p.onsProvider = onsProvider }),
	)
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

func (p *SDK) PostStart(ctx context.Context) error {
	if err := p.SDK.PostStart(ctx); err != nil {
		return err
	}

	if !p.OrionEnabled() {
		logger.Infof("Orion platform not enabled, skipping start")
		return nil
	}
	if p.onsProvider == nil {
		return errors.New("ons provider not set")
	}

	if err := p.onsProvider.Start(ctx); err != nil {
		return fmt.Errorf("failed starting orion network service provider: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := p.onsProvider.Stop(); err != nil {
			logger.Errorf("failed stopping orion network service provider [%s]", err)
		}
	}()
	return nil
}

func newOrionNetworkServiceProvider(in struct {
	dig.In
	KVSS          *kvs.KVS
	Publisher     events.Publisher
	Subscriber    events.Subscriber
	ConfigService driver.ConfigService
	Config        *core.Config
	Drivers       []driver3.NamedDriver `group:"db-drivers"`
}) (*core.ONSProvider, error) {
	return core.NewOrionNetworkServiceProvider(in.ConfigService, in.Config, in.KVSS, in.Publisher, in.Subscriber, in.Drivers)
}
