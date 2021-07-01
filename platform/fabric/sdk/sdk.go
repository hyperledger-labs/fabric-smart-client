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
	var err error
	fnspConfig, err := core.NewConfig(view2.GetConfigService(p.registry))
	assert.NoError(err, "failed parsing configuration")
	p.fnsProvider, err = core.NewFabricNetworkServiceProvider(p.registry, fnspConfig)
	assert.NoError(err, "failed instantiating fabric network service provider")
	assert.NoError(p.registry.RegisterService(p.fnsProvider))
	assert.NoError(fabric2.GetDefaultFNS(p.registry).ProcessorManager().SetDefaultProcessor(
		state.NewRWSetProcessor(fabric2.GetDefaultFNS(p.registry)),
	))

	// TODO: remove this
	assert.NoError(p.registry.RegisterService(tracker.NewTracker()))
	// TODO: change this
	assert.NoError(p.registry.RegisterService(vault.NewService(p.registry)))

	return nil
}

func (p *p) Start(ctx context.Context) error {
	if !view2.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping start")
		return nil
	}

	if err := p.fnsProvider.Start(ctx); err != nil {
		return errors.WithMessagef(err, "failed starting fabric network service provider")
	}

	return nil
}
