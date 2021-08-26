/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"bytes"

	"github.com/pkg/errors"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
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

type Discovery interface {
	Peers() []NetworkMember
}

type KVS interface {
	Exists(id string) bool
	Put(id string, state interface{}) error
	Get(id string, state interface{}) error
}

type endpointEntry struct {
	Endpoints map[driver.PortName]string
	Ephemeral view.Identity
	Identity  view.Identity
}

type service struct {
	sp           view2.ServiceProvider
	resolvers    []*resolver
	discovery    Discovery
	kvs          KVS
	pkiResolvers []driver.PKIResolver
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(sp view2.ServiceProvider, discovery Discovery, kvs KVS) (*service, error) {
	er := &service{
		sp:           sp,
		discovery:    discovery,
		kvs:          kvs,
		pkiResolvers: []driver.PKIResolver{},
	}
	return er, nil
}

func (r *service) Endpoint(party view.Identity) (map[driver.PortName]string, error) {
	cursor := party
	for {
		// root endpoints have addresses
		// is this a root endpoint
		e, err := r.rootEndpoint(cursor)
		if err != nil {
			logger.Debugf("resolving via binding for %s", cursor)
			ee, err := r.getBinding(cursor.UniqueID())
			if err != nil {
				return nil, errors.Wrapf(err, "endpoint not found for identity [%s,%s]", string(cursor), cursor.UniqueID())
			}

			cursor = ee.Identity
			logger.Debugf("continue to [%s,%s,%s]", cursor, ee.Endpoints, ee.Identity)
			continue
		}

		logger.Debugf("endpoint for [%s] to [%s] with ports [%v]", party, cursor, e)
		return e, nil
	}
}

var (
	resolved = make(map[string]view.Identity)
)

func (r *service) Resolve(party view.Identity) (view.Identity, map[driver.PortName]string, []byte, error) {
	cursor := party
	for {
		// root endpoints have addresses
		// is this a root endpoint
		e, err := r.rootEndpoint(cursor)
		if err != nil {
			logger.Debugf("resolving via binding for %s", cursor)
			ee, err := r.getBinding(cursor.UniqueID())
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, "endpoint not found for identity [%s,%s]", string(cursor), cursor.UniqueID())
			}

			cursor = ee.Identity
			logger.Debugf("continue to [%s,%s,%s]", cursor, ee.Endpoints, ee.Identity)
			continue
		}

		logger.Debugf("resolved [%s] to [%s] with ports [%v]", party, cursor, e)
		alreadyResolved, ok := resolved[party.UniqueID()]
		if ok && !alreadyResolved.Equal(cursor) {
			return nil, nil, nil, errors.Errorf("[%s] already resolved to [%s], resolved to [%s] this time", party, alreadyResolved, cursor)
		}
		resolved[party.UniqueID()] = cursor
		return cursor, e, r.pkiResolve(cursor), nil
	}
}

func (r *service) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	e, err := r.Endpoint(longTerm)
	if err != nil {
		return errors.Errorf("long term identity not found for identity [%s]", longTerm.UniqueID())
	}
	logger.Debugf("bind [%s] to [%s]", ephemeral.String(), longTerm.String())
	if err := r.putBinding(ephemeral.UniqueID(), &endpointEntry{Endpoints: e, Identity: longTerm, Ephemeral: ephemeral}); err != nil {
		return errors.WithMessagef(err, "failed storing binding of [%s]  to [%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	}

	return nil
}

func (r *service) IsBoundTo(a view.Identity, b view.Identity) bool {
	for {
		if a.Equal(b) {
			return true
		}
		next, err := r.getBinding(a.UniqueID())
		if err != nil {
			return false
		}
		if next.Identity.Equal(b) {
			return true
		}
		a = next.Identity
	}
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
	return nil, errors.Errorf("identity not found at [%s,%s]", endpoint, view.Identity(pkid))
}

func (r *service) AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	logger.Debugf("adding resolver [%s,%s,%v,%v,%s]", name, domain, addresses, aliases, view.Identity(id).String())

	// is there a resolver with the same name?
	for _, resolver := range r.resolvers {
		if resolver.Name == name {
			// TODO: perform additional checks

			// Then bind
			return resolver.Id, r.Bind(resolver.Id, id)
		}
	}

	r.resolvers = append(r.resolvers, &resolver{
		Name:      name,
		Domain:    domain,
		Addresses: addresses,
		Aliases:   aliases,
		Id:        id,
	})
	return nil, nil
}

func (r *service) AddPKIResolver(pkiResolver driver.PKIResolver) error {
	if pkiResolver == nil {
		return errors.New("pki resolver should not be nil")
	}
	r.pkiResolvers = append(r.pkiResolvers, pkiResolver)
	return nil
}

func (r *service) AddLongTermIdentity(identity view.Identity) error {
	return r.putBinding(identity.String(), &endpointEntry{
		Identity: identity,
	})
}

func (r *service) pkiResolve(id view.Identity) []byte {
	for _, pkiResolver := range r.pkiResolvers {
		if res := pkiResolver.GetPKIidOfCert(id); len(res) != 0 {
			logger.Debugf("pki resolved for [%s]", id)
			return res
		}
	}
	logger.Warnf("cannot resolve pki for [%s]", id)
	return nil
}

func (r *service) rootEndpoint(party view.Identity) (map[driver.PortName]string, error) {
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
	if err := r.kvs.Put(k, entry); err != nil {
		return err
	}
	return nil
}

func (r *service) getBinding(key string) (*endpointEntry, error) {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fsc.endpoint.binding",
		[]string{key},
	)
	if !r.kvs.Exists(k) {
		return nil, errors.Errorf("binding not found for [%s]", key)
	}
	entry := &endpointEntry{}
	if err := r.kvs.Get(k, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func convert(o map[string]string) map[driver.PortName]string {
	r := map[driver.PortName]string{}
	for k, v := range o {
		r[driver.PortName(k)] = v
	}
	return r
}
