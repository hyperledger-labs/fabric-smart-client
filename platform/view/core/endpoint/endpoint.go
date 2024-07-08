/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"bytes"
	"net"
	"reflect"
	"strings"
	"sync"

	"golang.org/x/exp/slices"

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
	Addresses      map[driver.PortName]string
	Aliases        []string
	PKI            []byte
	PKILock        sync.RWMutex
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
	resolvers      []*Resolver
	resolversMutex sync.RWMutex
	kvs            KVS

	pkiExtractorsLock      sync.RWMutex
	publicKeyExtractors    []driver.PublicKeyExtractor
	publicKeyIDSynthesizer driver.PublicKeyIDSynthesizer
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(kvs KVS) (*Service, error) {
	er := &Service{
		kvs:                    kvs,
		publicKeyExtractors:    []driver.PublicKeyExtractor{},
		publicKeyIDSynthesizer: DefaultPublicKeyIDSynthesizer{},
	}
	return er, nil
}

func (r *Service) Endpoint(party view.Identity) (map[driver.PortName]string, error) {
	_, e, _, err := r.resolve(party)
	return e, err
}

func (r *Service) Resolve(party view.Identity) (string, view.Identity, map[driver.PortName]string, []byte, error) {
	cursor, e, resolver, err := r.resolve(party)
	if err != nil {
		return "", nil, nil, nil, err
	}
	return resolver.Name, cursor, e, r.pkiResolve(resolver), nil
}

func (r *Service) resolve(party view.Identity) (view.Identity, map[driver.PortName]string, *Resolver, error) {
	cursor := party
	for {
		// root endpoints have addresses
		// is this a root endpoint
		resolver, e, err := r.rootEndpoint(cursor)
		if err == nil {
			return cursor, e, resolver, nil
		}
		logger.Debugf("resolving via binding for %s", cursor)
		cursor, err = r.getBinding(cursor)
		if err != nil {
			return nil, nil, nil, err
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
		resolverPKID := r.pkiResolve(resolver)
		found := false
		for _, addr := range resolver.Addresses {
			if endpoint == addr {
				found = true
				break
			}
		}
		if !found {
			// check aliases
			found = slices.Contains(resolver.Aliases, endpoint)
		}
		if endpoint == resolver.Name ||
			found ||
			endpoint == resolver.Name+"."+resolver.Domain ||
			bytes.Equal(pkID, resolver.Id) ||
			bytes.Equal(pkID, resolverPKID) {

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
			return resolver.Id, r.Bind(resolver.Id, id)
		}
		for _, alias := range resolver.Aliases {
			if slices.Contains(aliases, alias) {
				logger.Warnf("alias [%s] already defined by resolver [%s]", alias, resolver.Name)
			}
		}
	}
	r.resolversMutex.RUnlock()

	r.resolversMutex.Lock()
	defer r.resolversMutex.Unlock()

	// resolve addresses to their IPs, if needed
	for k, v := range addresses {
		addresses[k] = LookupIPv4(v)
	}
	r.resolvers = append(r.resolvers, &Resolver{
		Name:      name,
		Domain:    domain,
		Addresses: convert(addresses),
		Aliases:   aliases,
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
	resolver.PKILock.Lock()
	defer resolver.PKILock.Unlock()
	if len(resolver.PKI) != 0 {
		return resolver.PKI
	}

	resolver.PKI = r.ExtractPKI(resolver.Id)
	return resolver.PKI
}

func (r *Service) ExtractPKI(id []byte) []byte {
	r.pkiExtractorsLock.RLock()
	defer r.pkiExtractorsLock.RUnlock()

	for _, extractor := range r.publicKeyExtractors {
		if pk, err := extractor.ExtractPublicKey(id); pk != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pki resolved for [%s]", id)
			}
			return r.publicKeyIDSynthesizer.PublicKeyID(pk)
		} else {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pki not resolved by [%s] for [%s]: [%s]", getIdentifier(extractor), id, err)
			}
		}
	}
	logger.Warnf("cannot resolve pki for [%s]", id)
	return nil
}

func (r *Service) rootEndpoint(party view.Identity) (*Resolver, map[driver.PortName]string, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return resolver, resolver.Addresses, nil
		}
	}

	return nil, nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
}

func (r *Service) putBinding(ephemeral, longTerm view.Identity) error {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fsc.endpoint.binding",
		[]string{ephemeral.UniqueID()},
	)
	if err := r.kvs.Put(k, longTerm); err != nil {
		return err
	}
	return nil
}

func (r *Service) getBinding(ephemeral view.Identity) (view.Identity, error) {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fsc.endpoint.binding",
		[]string{ephemeral.UniqueID()},
	)
	if !r.kvs.Exists(k) {
		return nil, errors.Errorf("binding not found for [%s]", ephemeral.UniqueID())
	}
	longTerm := view.Identity{}
	if err := r.kvs.Get(k, &longTerm); err != nil {
		return nil, err
	}
	return longTerm, nil
}

var portNameMap = map[string]driver.PortName{
	strings.ToLower(string(driver.ListenPort)): driver.ListenPort,
	strings.ToLower(string(driver.ViewPort)):   driver.ViewPort,
	strings.ToLower(string(driver.P2PPort)):    driver.P2PPort,
}

func convert(o map[string]string) map[driver.PortName]string {
	r := map[driver.PortName]string{}
	for k, v := range o {
		r[portNameMap[strings.ToLower(k)]] = v
	}
	return r
}

func LookupIPv4(endpoint string) string {
	s := strings.Split(endpoint, ":")
	if len(s) < 2 {
		return endpoint
	}
	var addrS string
	addr, err := net.LookupIP(s[0])
	if err != nil {
		addrS = s[0]
	} else {
		addrS = addr[0].String()
	}
	port := s[1]
	return net.JoinHostPort(addrS, port)
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
