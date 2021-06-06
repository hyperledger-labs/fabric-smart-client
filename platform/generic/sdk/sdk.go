/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"context"
	"io/ioutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/sdk/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/services/endpoint"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
)

var logger = flogging.MustGetLogger("generic-sdk.platform.generic")

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
	configProvider := view2.GetConfigService(p.registry)
	if !configProvider.GetBool("generic.enabled") {
		logger.Infof("Generic platform not enabled, skipping")
		return nil
	}
	logger.Infof("Generic platform enabled, installing...")

	assert.NoError(p.registry.RegisterService(crypto.NewProvider()))

	logger.Infof("Set Endpoint Service")
	es, err := view.NewEndpointService(p.registry)
	assert.NoError(err, "failed wrapping endpoint service")
	resolverService, err := endpoint.NewResolverService(view2.GetConfigService(p.registry), es)
	assert.NoError(err, "failed instantiating fabric endpoint resolver")
	assert.NoError(resolverService.LoadResolvers(), "failed loading fabric endpoint resolvers")

	logger.Infof("Set Identity Service")
	idProvider, err := id.NewProvider(p.registry)
	assert.NoError(err, "failed creating id provider")
	assert.NoError(p.registry.RegisterService(idProvider))

	id, verifier, err := ecdsa.NewIdentityFromPEMCert(idProvider.DefaultIdentity())
	assert.NoError(err, "failed loading default verifier")
	fileCont, err := ioutil.ReadFile(configProvider.GetPath("generic.identity.key.file"))
	assert.NoError(err, "failed reading file [%s]", fileCont)
	signer, err := ecdsa.NewSignerFromPEM(fileCont)
	assert.NoError(err, "failed loading default signer")
	assert.NoError(err, view2.GetSigService(p.registry).RegisterSignerWithType(view2.ECDSAIdentity, id, signer, verifier))

	return nil
}

func (p *p) Start(ctx context.Context) error {
	return nil
}
