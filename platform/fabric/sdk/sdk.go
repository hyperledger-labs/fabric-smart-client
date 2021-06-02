/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"context"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/crypto"
	endpoint2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/impl"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
)

var logger = flogging.MustGetLogger("fabric-sdk")

type Registry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type p struct {
	registry Registry
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

	logger.Infof("Set Endpoint Service")
	endpointService, err := endpoint2.NewService(p.registry, nil, endpoint2.NewPKIResolver())
	if err != nil {
		return errors.Wrap(err, "failed instantiating endpoint resolver")
	}
	assert.NoError(p.registry.RegisterService(endpointService))

	logger.Infof("Set Fabric Network Service Provider")
	fnsProvider, err := core.NewFabricNetworkServiceProvider(p.registry)
	if err != nil {
		return errors.Wrap(err, "failed instantiating fabric network service provider")
	}
	assert.NoError(p.registry.RegisterService(fnsProvider))
	assert.NoError(fabric2.GetDefaultNetwork(p.registry).ProcessorManager().SetDefaultProcessor(
		state.NewRWSetProcessor(fabric2.GetDefaultNetwork(p.registry)),
	))

	// id provider
	logger.Infof("Set Identity Service")
	idProvider, err := id.NewProvider(p.registry, fabric2.GetDefaultNetwork(p.registry).IdentityProvider().DefaultIdentity())
	if err != nil {
		return errors.Wrap(err, "failed creating id provider")
	}
	assert.NoError(p.registry.RegisterService(idProvider))

	// TODO: remove this
	assert.NoError(p.registry.RegisterService(tracker.NewTracker()))
	// TODO: change this
	assert.NoError(p.registry.RegisterService(impl.NewWorldStateService(p.registry)))

	return nil
}

func (p *p) Start(ctx context.Context) error {
	if !view2.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping start")
		return nil
	}

	// TODO: add listener to fabric service when a channel is opened.
	for _, ch := range fabric2.GetDefaultNetwork(p.registry).Channels() {
		fabric2.GetChannelDefaultNetwork(p.registry, ch)
	}

	return nil
}
