/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/pkg/errors"

	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
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

type p struct {
	registry    Registry
	fnsProvider Startable
}

func NewSDK(registry Registry) *p {
	return &p{registry: registry}
}

func (p *p) Install() error {
	if !view2.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping")
		return nil
	}
	logger.Infof("Fabric platform enabled, installing...")

	cryptoProvider := crypto.NewProvider()
	assert.NoError(p.registry.RegisterService(cryptoProvider))

	logger.Infof("Set Fabric Network Service Provider")
	fnsConfig, err := core.NewConfig(view2.GetConfigService(p.registry))
	assert.NoError(err, "failed parsing configuration")
	p.fnsProvider, err = core.NewFabricNetworkServiceProvider(p.registry, fnsConfig)
	assert.NoError(err, "failed instantiating fabric network service provider")
	assert.NoError(p.registry.RegisterService(p.fnsProvider))
	assert.NoError(p.registry.RegisterService(fabric2.NewNetworkServiceProvider(p.registry)))

	// Register processors
	names := fabric2.GetFabricNetworkNames(p.registry)
	if len(names) == 0 {
		return errors.New("no fabric network names found")
	}
	for _, name := range names {
		fns := fabric2.GetFabricNetworkService(p.registry, name)
		if fns == nil {
			return errors.Errorf("no fabric network service found for [%s]", name)
		}
		assert.NoError(fns.ProcessorManager().SetDefaultProcessor(
			state.NewRWSetProcessor(fabric2.GetDefaultFNS(p.registry)),
		), "failed setting state processor for fabric network [%s]", name)
	}

	assert.NotNil(fabric2.GetDefaultFNS(p.registry), "default fabric network service not found")

	// TODO: remove this
	assert.NoError(p.registry.RegisterService(tracker.NewTracker()))
	// TODO: change this
	assert.NoError(p.registry.RegisterService(vault.NewService(p.registry)))

	// weaver provider
	assert.NoError(p.registry.RegisterService(weaver.NewProvider()))

	return nil
}

func (p *p) Start(ctx context.Context) error {
	return nil
}

func (p *p) PostStart(ctx context.Context) error {
	if !view2.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping start")
		return nil
	}

	// start the delivery pipeline on all configured networks
	fnsConfig, err := core.NewConfig(view2.GetConfigService(p.registry))
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
