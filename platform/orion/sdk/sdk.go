/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/orion/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
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
	registry    Registry
	onsProvider Startable
}

func NewSDK(registry Registry) *SDK {
	return &SDK{registry: registry}
}

func (p *SDK) Install() error {
	if !view.GetConfigService(p.registry).GetBool("orion.enabled") {
		logger.Infof("Orion platform not enabled, skipping")
		return nil
	}
	logger.Infof("Orion platform enabled, installing...")

	logger.Infof("Set Orion Network Service Provider")
	var err error
	onspConfig, err := core.NewConfig(view.GetConfigService(p.registry))
	assert.NoError(err, "failed parsing configuration")
	p.onsProvider, err = core.NewOrionNetworkServiceProvider(p.registry, onspConfig)
	assert.NoError(err, "failed instantiating orion network service provider")
	assert.NoError(p.registry.RegisterService(p.onsProvider))
	assert.NoError(p.registry.RegisterService(orion.NewNetworkServiceProvider(p.registry)))

	// Install finality handler
	finality.GetManager(p.registry).AddHandler(&FinalityHandler{sp: p.registry})

	return nil
}

func (p *SDK) Start(context.Context) error {
	return nil
}

func (p *SDK) PostStart(ctx context.Context) error {
	if !view.GetConfigService(p.registry).GetBool("orion.enabled") {
		logger.Infof("Orion platform not enabled, skipping start")
		return nil
	}

	if err := p.onsProvider.Start(ctx); err != nil {
		return errors.WithMessagef(err, "failed starting orion network service provider")
	}

	go func() {
		<-ctx.Done()
		if err := p.onsProvider.Stop(); err != nil {
			logger.Errorf("failed stopping orion network service provider [%s]", err)
		}
	}()
	return nil
}

type FinalityHandler struct {
	sp view.ServiceProvider
}

func (f *FinalityHandler) IsFinal(ctx context.Context, network, channel, txID string) error {
	ons := orion.GetOrionNetworkService(f.sp, network)
	if ons != nil {
		return ons.Finality().IsFinal(ctx, txID)
	}
	return errors.Errorf("cannot find orio network [%s]", network)
}
