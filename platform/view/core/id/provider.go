/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package id

import (
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ConfigProvider interface {
	GetPath(s string) string
}

type SigService interface {
	RegisterSignerWithType(typ api.IdentityType, identity view.Identity, signer api.Signer, verifier api.Verifier) error
}

type EndpointService interface {
	GetIdentity(label string, pkid []byte) (view.Identity, error)
}

type provider struct {
	configProvider  ConfigProvider
	sigService      SigService
	endpointService EndpointService
	defaultID       view.Identity
}

func NewProvider(configProvider ConfigProvider, sigService SigService, endpointService EndpointService, defaultID view.Identity) *provider {
	return &provider{configProvider: configProvider, sigService: sigService, endpointService: endpointService, defaultID: defaultID}
}

func (p *provider) LoadIdentities() error {
	defaultID, err := LoadIdentity(p.configProvider.GetPath("fsc.identity.cert.file"))
	if err != nil {
		return errors.Wrapf(err, "failed loading SFC Node Identity")
	}
	id, verifier, err := ecdsa.NewIdentityFromPEMCert(defaultID)
	if err != nil {
		return errors.Wrap(err, "failed loading default verifier")
	}
	fileCont, err := ioutil.ReadFile(p.configProvider.GetPath("fsc.identity.key.file"))
	if err != nil {
		return errors.Wrapf(err, "failed reading file [%s]", fileCont)
	}
	signer, err := ecdsa.NewSignerFromPEM(fileCont)
	if err != nil {
		return errors.Wrapf(err, "failed loading default signer")
	}
	if err := p.sigService.RegisterSignerWithType(api.ECDSAIdentity, id, signer, verifier); err != nil {
		return errors.Wrapf(err, "failed registering default identity signer")
	}
	p.defaultID = defaultID
	return nil
}

func (p *provider) DefaultIdentity() view.Identity {
	return p.defaultID
}

func (p *provider) Identity(label string) view.Identity {
	id, err := p.endpointService.GetIdentity(label, nil)
	if err != nil {
		panic(err)
	}
	return id
}
