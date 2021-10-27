/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"context"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
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
	if !view2.GetConfigService(p.registry).GetBool("orion.enabled") {
		logger.Infof("Orion platform not enabled, skipping")
		return nil
	}
	logger.Infof("Orion platform enabled, installing...")

	logger.Infof("Set Orion Network Service Provider")
	var err error
	onspConfig, err := core.NewConfig(view2.GetConfigService(p.registry))
	assert.NoError(err, "failed parsing configuration")
	p.onsProvider, err = core.NewOrionNetworkServiceProvider(p.registry, onspConfig)
	assert.NoError(err, "failed instantiating orion network service provider")
	assert.NoError(p.registry.RegisterService(p.onsProvider))

	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	if !view2.GetConfigService(p.registry).GetBool("orion.enabled") {
		logger.Infof("Orion platform not enabled, skipping start")
		return nil
	}

	if err := p.onsProvider.Start(ctx); err != nil {
		return errors.WithMessagef(err, "failed starting orion network service provider")
	}

	go func() {
		select {
		case <-ctx.Done():
			if err := p.onsProvider.Stop(); err != nil {
				logger.Errorf("failed stopping orion network service provider [%s]", err)
			}
		}
	}()

	return nil
}
