/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endpoint

import (
	"bytes"
	"encoding/base64"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/core/id"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("generic-sdk.platform.generic.endpoint")

type IdentityConf struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
}

type resolver struct {
	Name           string            `yaml:"name,omitempty"`
	Domain         string            `yaml:"domain,omitempty"`
	Identity       IdentityConf      `yaml:"identity,omitempty"`
	Addresses      map[string]string `yaml:"addresses,omitempty"`
	Id             []byte
	IdentityGetter func() (view.Identity, error)
}

func (r *resolver) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		return r.IdentityGetter()
	}

	return r.Id, nil
}

// NetworkMember is a peer's representation
type NetworkMember struct {
	Endpoint         string
	Metadata         []byte
	PKIid            []byte
	InternalEndpoint string
}

type PKIResolver interface {
	GetPKIidOfCert(peerIdentity view.Identity) []byte
}

type Discovery interface {
	Peers() []NetworkMember
}

type SignerService interface {
	RegisterSigner(identity view.Identity, signer api.Signer, verifier api.Verifier) error
}

type endpointEntry struct {
	Endpoints map[api.PortName]string
	Ephemeral view.Identity
	Identity  view.Identity
}

type service struct {
	sp          view2.ServiceProvider
	resolvers   []*resolver
	discovery   Discovery
	pkiResolver PKIResolver

	EndpointBindings map[string]endpointEntry
}

func NewService(sp view2.ServiceProvider, discovery Discovery, pkiResolver PKIResolver) (*service, error) {
	// Load resolver
	var resolvers []*resolver
	configProvider := view2.GetConfigService(sp)

	if configProvider.IsSet("generic.endpoint.resolves") {
		logger.Infof("loading resolvers")
		err := configProvider.UnmarshalKey("generic.endpoint.resolves", &resolvers)
		if err != nil {
			logger.Errorf("failed loading resolves [%s]", err)
			return nil, err
		}
		logger.Infof("loaded resolves successfully, number of entries found %d", len(resolvers))

		for _, resolver := range resolvers {
			// Load identity
			raw, err := id.Serialize(configProvider.TranslatePath(resolver.Identity.Path))
			if err != nil {
				return nil, err
			}
			resolver.Id = raw
			logger.Infof("resolver [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.Id).UniqueID(),
			)
		}
	}

	return &service{
		sp:               sp,
		resolvers:        resolvers,
		discovery:        discovery,
		pkiResolver:      pkiResolver,
		EndpointBindings: map[string]endpointEntry{},
	}, nil
}

func (r *service) Endpoint(party view.Identity) (map[api.PortName]string, error) {
	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return convert(resolver.Addresses), nil
		}
	}
	// lookup for the binding
	endpointEntry, ok := r.EndpointBindings[party.UniqueID()]
	if !ok {
		return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
	}
	return endpointEntry.Endpoints, nil
}

func (r *service) Resolve(party view.Identity) (view.Identity, map[api.PortName]string, []byte, error) {

	e, err := r.endpointInternal(party)
	if err != nil {
		logger.Debugf("resolving via binding for %s", base64.StdEncoding.EncodeToString(r.pkiResolver.GetPKIidOfCert(party)))
		endpointEntry, ok := r.EndpointBindings[party.UniqueID()]
		if !ok {
			return nil, nil, nil, errors.Errorf("endpoint not found for identity [%s]", party.UniqueID())
		}

		if len(endpointEntry.Endpoints) != 0 {
			e = endpointEntry.Endpoints
		} else {
			e, err = r.Endpoint(endpointEntry.Identity)
			if err != nil {
				return nil, nil, nil, errors.Errorf("endpoint not found for identity [%s]", party.UniqueID())
			}
		}

		logger.Debugf("resolved to [%s,%s,%s]",
			base64.StdEncoding.EncodeToString(r.pkiResolver.GetPKIidOfCert(party)),
			e,
			r.pkiResolver.GetPKIidOfCert(endpointEntry.Identity),
		)
		return endpointEntry.Identity, e, r.pkiResolver.GetPKIidOfCert(endpointEntry.Identity), nil
	}

	return party, e, r.pkiResolver.GetPKIidOfCert(party.Bytes()), nil
}

func (r *service) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	e, err := r.Endpoint(longTerm)
	if err != nil {
		return errors.Errorf("long term identity not found for identity [%s]", longTerm.UniqueID())
	}
	r.EndpointBindings[ephemeral.UniqueID()] = endpointEntry{
		Endpoints: e, Identity: longTerm, Ephemeral: ephemeral,
	}

	return nil
}

func (r *service) GetIdentity(endpoint string, pkid []byte) (view.Identity, error) {
	for _, resolver := range r.resolvers {
		resolverPKID := r.pkiResolver.GetPKIidOfCert(resolver.Id)

		found := false
		for _, addr := range resolver.Addresses {
			if endpoint == addr {
				found = true
				break
			}
		}

		if endpoint == resolver.Name ||
			found ||
			endpoint == resolver.Name+"."+resolver.Domain ||
			bytes.Equal(pkid, resolver.Id) ||
			bytes.Equal(pkid, resolverPKID) {

			id, err := resolver.GetIdentity()
			if err != nil {
				return nil, err
			}
			logger.Infof("resolving [%s,%s] to %s", endpoint,
				base64.StdEncoding.EncodeToString(hash.SHA256OrPanic(pkid)),
				base64.StdEncoding.EncodeToString(hash.SHA256OrPanic(id)))
			return id, nil
		}
	}

	return nil, errors.Errorf("identity not found at [%s,%s] %s", endpoint, base64.StdEncoding.EncodeToString(pkid), debug.Stack())
}

func (r *service) endpointInternal(party view.Identity) (map[api.PortName]string, error) {
	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return convert(resolver.Addresses), nil
		}
	}
	return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
}

func (r *service) AddLongTermIdentity(identity view.Identity) {
	r.EndpointBindings[identity.String()] = endpointEntry{
		Identity: identity,
	}
}

func convert(o map[string]string) map[api.PortName]string {
	r := map[api.PortName]string{}
	for k, v := range o {
		r[api.PortName(k)] = v
	}
	return r
}
