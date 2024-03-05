/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"bytes"
	"reflect"
	"sync"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("view-sdk.endpoint")

type Resolver struct {
	Name           string
	Domain         string
	Addresses      []AddressSet
	Aliases        Set[string]
	PKI            []byte
	Id             []byte
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *Resolver) GetIdentity() (view.Identity, error) {
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

type Service struct {
	sp             view2.ServiceProvider
	resolvers      []*Resolver
	resolversMutex sync.RWMutex
	discovery      Discovery
	kvs            KVS

	pkiExtractorsLock      sync.RWMutex
	publicKeyExtractors    []driver.PublicKeyExtractor
	publicKeyIDSynthesizer driver.PublicKeyIDSynthesizer
	endpointSelector       SelectionStrategy[AddressSet]
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(sp view2.ServiceProvider, discovery Discovery, kvs KVS) (*Service, error) {
	er := &Service{
		sp:                     sp,
		discovery:              discovery,
		kvs:                    kvs,
		publicKeyExtractors:    []driver.PublicKeyExtractor{},
		publicKeyIDSynthesizer: DefaultPublicKeyIDSynthesizer{},
		endpointSelector:       AlwaysFirst[AddressSet](),
	}
	return er, nil
}

func (r *Service) Resolve(party view.Identity) (view.Identity, AddressSet, []byte, error) {
	resolver, err := r.resolve(party)
	if err != nil {
		return nil, nil, nil, err
	}
	endpoint := r.endpointSelector(resolver.Addresses)
	return resolver.Id, endpoint, r.pkiResolve(resolver), nil
}

func (r *Service) resolve(party view.Identity) (*Resolver, error) {
	cursor := party
	for {
		// root endpoints have addresses
		// is this a root endpoint
		resolver, err := r.rootEndpoint(cursor)
		if err == nil {
			return resolver, nil
		}
		logger.Debugf("resolving via binding for %s", cursor)
		cursor, err = r.getBinding(cursor)
		if err != nil {
			return nil, err
		}
		logger.Debugf("continue to [%s]", cursor)
	}
}

func (r *Service) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	if longTerm.Equal(ephemeral) {
		logger.Debugf("cannot bind [%s] to [%s], they are the same", longTerm, ephemeral)
		return nil
	}

	logger.Debugf("bind [%s] to [%s]", ephemeral, longTerm)

	if err := r.putBinding(ephemeral, longTerm); err != nil {
		return errors.WithMessagef(err, "failed storing binding of [%s]  to [%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	}

	return nil
}

func (r *Service) IsBoundTo(a view.Identity, b view.Identity) bool {
	for {
		if a.Equal(b) {
			return true
		}
		next, err := r.getBinding(a)
		if err != nil {
			return false
		}
		if next.Equal(b) {
			return true
		}
		a = next
	}
}

func (r *Service) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// search in the resolver list
	for _, resolver := range r.resolvers {
		if endpoint == resolver.Name ||
			resolver.Aliases.Contains(endpoint) ||
			endpoint == resolver.Name+"."+resolver.Domain ||
			contains(resolver.Addresses, endpoint) ||
			bytes.Equal(pkID, resolver.Id) ||
			bytes.Equal(pkID, r.pkiResolve(resolver)) {

			id, err := resolver.GetIdentity()
			if err != nil {
				return nil, err
			}
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("resolving [%s,%s] to %s", endpoint, view.Identity(pkID), id)
			}
			return id, nil
		}
	}
	return nil, errors.Errorf("identity not found at [%s,%s]", endpoint, view.Identity(pkID))
}

func (r *Service) AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("adding resolver [%s,%s,%v,%v,%s]", name, domain, addresses, aliases, view.Identity(id).String())
	}

	// is there a resolver with the same name or clashing aliases?
	r.resolversMutex.RLock()
	for _, resolver := range r.resolvers {
		if resolver.Name == name {
			// TODO: perform additional checks

			// Then bind
			r.resolversMutex.RUnlock()
			r.resolversMutex.Lock()
			resolver.Addresses = append(resolver.Addresses, newAddressSet(addresses))
			r.resolversMutex.Unlock()
			return resolver.Id, r.Bind(resolver.Id, id)
		}
		for _, alias := range aliases {
			if resolver.Aliases.Contains(alias) {
				logger.Warnf("alias [%s] already defined by resolver [%s]", alias, resolver.Name)
			}
		}
	}
	r.resolversMutex.RUnlock()

	r.resolversMutex.Lock()
	defer r.resolversMutex.Unlock()

	// resolve addresses to their IPs, if needed
	r.resolvers = append(r.resolvers, &Resolver{
		Name:      name,
		Domain:    domain,
		Addresses: []AddressSet{newAddressSet(addresses)},
		Aliases:   newSet(aliases),
		Id:        id,
	})
	return nil, nil
}

func (r *Service) AddPublicKeyExtractor(publicKeyExtractor driver.PublicKeyExtractor) error {
	r.pkiExtractorsLock.Lock()
	defer r.pkiExtractorsLock.Unlock()

	if publicKeyExtractor == nil {
		return errors.New("pki resolver should not be nil")
	}
	r.publicKeyExtractors = append(r.publicKeyExtractors, publicKeyExtractor)
	return nil
}

func (r *Service) SetPublicKeyIDSynthesizer(publicKeyIDSynthesizer driver.PublicKeyIDSynthesizer) {
	r.publicKeyIDSynthesizer = publicKeyIDSynthesizer
}

func (r *Service) pkiResolve(resolver *Resolver) []byte {
	r.pkiExtractorsLock.RLock()
	if len(resolver.PKI) != 0 {
		r.pkiExtractorsLock.RUnlock()
		return resolver.PKI
	}
	r.pkiExtractorsLock.RUnlock()

	r.pkiExtractorsLock.Lock()
	defer r.pkiExtractorsLock.Unlock()
	for _, extractor := range r.publicKeyExtractors {
		if pk, err := extractor.ExtractPublicKey(resolver.Id); pk != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pki resolved for [%s]", resolver.Id)
			}
			resolver.PKI = r.publicKeyIDSynthesizer.PublicKeyID(pk)
			return resolver.PKI
		} else {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pki not resolved by [%s] for [%s]: [%s]", getIdentifier(extractor), resolver.Id, err)
			}
		}
	}
	logger.Warnf("cannot resolve pki for [%s]", resolver.Id)
	return nil
}

func (r *Service) rootEndpoint(party view.Identity) (*Resolver, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return resolver, nil
		}
	}

	return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
}

func (r *Service) putBinding(ephemeral, longTerm view.Identity) error {
	if err := r.kvs.Put(key(ephemeral), longTerm); err != nil {
		return err
	}
	return nil
}

func (r *Service) getBinding(ephemeral view.Identity) (view.Identity, error) {
	k := key(ephemeral)
	if !r.kvs.Exists(k) {
		return nil, errors.Errorf("binding not found for [%s]", ephemeral.UniqueID())
	}
	longTerm := view.Identity{}
	if err := r.kvs.Get(k, &longTerm); err != nil {
		return nil, err
	}
	return longTerm, nil
}

func key(id view.Identity) string {
	return kvs.CreateCompositeKeyOrPanic("platform.fsc.endpoint.binding", []string{id.UniqueID()})
}

func getIdentifier(f any) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "/" + t.Name()
}
