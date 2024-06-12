/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	finality2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger/fabric-protos-go/common"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabric-sdk")

type Registry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type Startable interface {
	Start(ctx context.Context) error
	Stop() error
}

type SDK struct {
	registry    Registry
	fnsProvider Startable
}

func NewSDK(registry Registry) *SDK {
	return &SDK{registry: registry}
}

func (p *SDK) Install() error {
	if !view.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping")
		return nil
	}
	logger.Infof("Fabric platform enabled, installing...")

	cryptoProvider := crypto.NewProvider()
	err := p.registry.RegisterService(cryptoProvider)
	assert.NoError(err)

	logger.Infof("Set Fabric Network Service Provider")
	fnsProvider, err := core.NewFabricNetworkServiceProvider(p.registry, view.GetConfigService(p.registry))
	p.fnsProvider = fnsProvider
	assert.NoError(err, "failed instantiating fabric network service provider")
	assert.NoError(p.registry.RegisterService(p.fnsProvider))
	subscriber, err := events.GetSubscriber(p.registry)
	assert.NoError(err, "failed to get subscriber")
	assert.NoError(p.registry.RegisterService(fabric.NewNetworkServiceProvider(fnsProvider, subscriber)))

	// Register processors
	names, err := fabric.GetFabricNetworkNames(p.registry)
	assert.NoError(err)
	if len(names) == 0 {
		return errors.New("no fabric network names found")
	}
	for _, name := range names {
		fns, err := fabric.GetFabricNetworkService(p.registry, name)
		assert.NoError(err, "no fabric network service found for [%s]", name)
		assert.NoError(fns.ProcessorManager().SetDefaultProcessor(state.NewRWSetProcessor(fns)), "failed setting state processor for fabric network [%s]", name)

		fsn, err := fnsProvider.FabricNetworkService(name)
		assert.NoError(err, "failed getting fabric network service for [%s]", name)
		for _, channelName := range fsn.ConfigService().ChannelIDs() {
			ch, err := fsn.Channel(channelName)
			assert.NoError(err)
			assert.NoError(ch.RWSetLoader().AddHandlerProvider(common.HeaderType_ENDORSER_TRANSACTION, rwset.NewEndorserTransactionHandler))
		}
	}
	_, err = fabric.GetDefaultFNS(p.registry)
	assert.NoError(err, "default fabric network service not found")

	// TODO: change this
	assert.NoError(p.registry.RegisterService(vault.NewService(p.registry)))

	// weaver provider
	assert.NoError(p.registry.RegisterService(weaver.NewProvider()))

	// Install finality handler
	fns, err := fabric.GetNetworkServiceProvider(p.registry)
	assert.NoError(err)
	finality.GetManager(p.registry).AddHandler(finality2.NewHandler(fns))

	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	return nil
}

func (p *SDK) PostStart(ctx context.Context) error {
	if !view.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping start")
		return nil
	}

	// start the delivery pipeline on all configured networks
	fnsConfig, err := core.NewConfig(view.GetConfigService(p.registry))
	assert.NoError(err, "failed parsing configuration")

	fnsConfig.Names()
	if err := p.fnsProvider.Start(ctx); err != nil {
		return errors.WithMessagef(err, "failed starting fabric network service provider")
	}

	go func() {
		<-ctx.Done()
		if err := p.fnsProvider.Stop(); err != nil {
			logger.Errorf("failed stopping fabric network service provider [%s]", err)
		}
	}()

	return nil
}
