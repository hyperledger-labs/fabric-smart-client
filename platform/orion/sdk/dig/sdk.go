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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	finality2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

var logger = logging.MustGetLogger("orion-sdk")

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
		p.Container().Provide(committer.NewFinalityListenerManagerProvider[driver2.ValidationCode], dig.As(new(driver2.ListenerManagerProvider))),
		p.Container().Provide(newNetworkConfigProvider, dig.As(new(driver2.NetworkConfigProvider))),
		p.Container().Provide(orion.NewNetworkServiceProvider),
		p.Container().Provide(digutils.Identity[*core.ONSProvider](), dig.As(new(driver2.OrionNetworkServiceProvider))),
		p.Container().Provide(finality2.NewHandler, dig.Group("finality-handlers")),
		p.Container().Provide(NewMetadataStore, dig.As(new(driver2.MetadataStore))),
		p.Container().Provide(NewEndorseTxStore, dig.As(new(driver2.EndorseTxStore))),
		p.Container().Provide(NewEnvelopeStore, dig.As(new(driver2.EnvelopeStore))),
	)
	if err != nil {
		return err
	}

	return p.SDK.Install()
}

func (p *SDK) Start(ctx context.Context) error {
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

func newNetworkConfigProvider() driver2.NetworkConfigProvider {
	return generic.NewStaticNetworkConfigProvider(generic.NewNetworkConfig())
}

func newOrionNetworkServiceProvider(in struct {
	dig.In
	EndorseTxKVS            driver2.EndorseTxStore
	MetadataKVS             driver2.MetadataStore
	EnvelopeKVS             driver2.EnvelopeStore
	Publisher               events.Publisher
	Subscriber              events.Subscriber
	ConfigService           driver.ConfigService
	Config                  *core.Config
	MetricsProvider         metrics.Provider
	TracerProvider          trace.TracerProvider
	Drivers                 []driver3.NamedDriver `group:"db-drivers"`
	NetworkConfigProvider   driver2.NetworkConfigProvider
	ListenerManagerProvider driver2.ListenerManagerProvider
}) (*core.ONSProvider, error) {
	return core.NewOrionNetworkServiceProvider(in.ConfigService, in.Config, in.EndorseTxKVS, in.MetadataKVS, in.EnvelopeKVS, in.Publisher, in.Subscriber, in.MetricsProvider, in.TracerProvider, in.Drivers, in.NetworkConfigProvider, in.ListenerManagerProvider)
}
