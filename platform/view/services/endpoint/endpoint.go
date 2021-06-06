/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endpoint

import (
	"bytes"
	"runtime/debug"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("view-sdk.endpoint")

type resolver struct {
	Name           string
	Domain         string
	Addresses      map[string]string
	Aliases        []string
	Id             []byte
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *resolver) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		id, _, err := r.IdentityGetter()
		return id, err
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

type endpointEntry struct {
	Endpoints map[api.PortName]string
	Ephemeral view.Identity
	Identity  view.Identity
}

type service struct {
	sp           view2.ServiceProvider
	resolvers    []*resolver
	Discovery    Discovery
	pkiResolvers []PKIResolver
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(sp view2.ServiceProvider, discovery Discovery) (*service, error) {
	er := &service{
		sp:           sp,
		Discovery:    discovery,
		pkiResolvers: []PKIResolver{},
	}
	return er, nil
}

func (r *service) Endpoint(party view.Identity) (map[api.PortName]string, error) {
	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return convert(resolver.Addresses), nil
		}
	}
	// lookup for the binding
	endpointEntry, err := r.getBinding(party.UniqueID())
	if err != nil {
		return nil, errors.Wrapf(err, "endpoint not found for identity %s", party.UniqueID())
	}
	return endpointEntry.Endpoints, nil
}

func (r *service) Resolve(party view.Identity) (view.Identity, map[api.PortName]string, []byte, error) {
	e, err := r.endpointInternal(party)
	if err != nil {
		logger.Debugf("resolving via binding for %s", party)
		ee, err := r.getBinding(party.UniqueID())
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "endpoint not found for identity [%s,%s]", string(party), party.UniqueID())
		}

		if len(ee.Endpoints) != 0 {
			e = ee.Endpoints
		} else {
			e, err = r.Endpoint(ee.Identity)
			if err != nil {
				return nil, nil, nil, errors.Errorf("endpoint not found for identity [%s]", party.UniqueID())
			}
		}

		logger.Debugf("resolved to [%s,%s,%s]", party, e, ee.Identity)

		return ee.Identity, e, r.pkiResolve(ee.Identity), nil
	}

	return party, e, r.pkiResolve(party.Bytes()), nil
}

func (r *service) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	e, err := r.Endpoint(longTerm)
	if err != nil {
		return errors.Errorf("long term identity not found for identity [%s]", longTerm.UniqueID())
	}
	if err := r.putBinding(ephemeral.UniqueID(), &endpointEntry{Endpoints: e, Identity: longTerm, Ephemeral: ephemeral}); err != nil {
		return errors.WithMessagef(err, "failed storing binding of [%s]  to [%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	}

	return nil
}

func (r *service) GetIdentity(endpoint string, pkid []byte) (view.Identity, error) {
	// search in the resolver list
	for _, resolver := range r.resolvers {
		resolverPKID := r.pkiResolve(resolver.Id)
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
			logger.Infof("resolving [%s,%s] to %s", endpoint, view.Identity(pkid), id)
			return id, nil
		}
	}
	// ask the msp service
	id, err := fabric.GetDefaultNetwork(r.sp).LocalMembership().GetIdentityByID(endpoint)
	if err != nil {
		return nil, errors.Errorf("identity not found at [%s,%s] %s [%s]", endpoint, view.Identity(pkid), debug.Stack(), err)
	}
	return id, nil
}

func (r *service) AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) error {
	r.resolvers = append(r.resolvers, &resolver{
		Name:      name,
		Domain:    domain,
		Addresses: addresses,
		Aliases:   aliases,
		Id:        id,
	})
	return nil
}

func (r *service) AddLongTermIdentity(identity view.Identity) error {
	return r.putBinding(identity.String(), &endpointEntry{
		Identity: identity,
	})
}

func (r *service) AddPKIResolver(pkiResolver PKIResolver) error {
	if pkiResolver == nil {
		return errors.New("pki resolver should not be nil")
	}
	r.pkiResolvers = append(r.pkiResolvers, pkiResolver)
	return nil
}

func (r *service) pkiResolve(id view.Identity) []byte {
	for _, pkiResolver := range r.pkiResolvers {
		if res := pkiResolver.GetPKIidOfCert(id); len(res) != 0 {
			return res
		}
	}
	return nil
}

func (r *service) endpointInternal(party view.Identity) (map[api.PortName]string, error) {
	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return convert(resolver.Addresses), nil
		}
	}
	return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
}

func (r *service) putBinding(key string, entry *endpointEntry) error {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fsc.endpoint.binding",
		[]string{key},
	)
	kvss := kvs.GetService(r.sp)
	if err := kvss.Put(k, entry); err != nil {
		return err
	}
	return nil
}

func (r *service) getBinding(key string) (*endpointEntry, error) {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fsc.endpoint.binding",
		[]string{key},
	)
	kvss := kvs.GetService(r.sp)
	if !kvss.Exists(k) {
		return nil, errors.Errorf("binding not found for [%s]", key)
	}
	entry := &endpointEntry{}
	if err := kvss.Get(k, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func convert(o map[string]string) map[api.PortName]string {
	r := map[api.PortName]string{}
	for k, v := range o {
		r[api.PortName(k)] = v
	}
	return r
}
