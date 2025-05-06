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

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"golang.org/x/exp/slices"
)

var logger = logging.MustGetLogger("view-sdk.endpoint")

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

func (r *Resolver) GetName() string { return r.Name }

func (r *Resolver) GetId() view.Identity { return r.Id }

func (r *Resolver) GetAddress(port driver.PortName) string { return r.Addresses[port] }

func (r *Resolver) GetAddresses() map[driver.PortName]string { return r.Addresses }

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

type Service struct {
	resolvers      []*Resolver
	resolversMutex sync.RWMutex
	bindingKVS     driver2.BindingStore

	pkiExtractorsLock      sync.RWMutex
	publicKeyExtractors    []driver.PublicKeyExtractor
	publicKeyIDSynthesizer driver.PublicKeyIDSynthesizer
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(bindingKVS driver2.BindingStore) (*Service, error) {
	er := &Service{
		bindingKVS:             bindingKVS,
		publicKeyExtractors:    []driver.PublicKeyExtractor{},
		publicKeyIDSynthesizer: DefaultPublicKeyIDSynthesizer{},
	}
	return er, nil
}

func (r *Service) Resolve(party view.Identity) (driver.Resolver, []byte, error) {
	resolver, err := r.resolver(party)
	if err != nil {
		return nil, nil, err
	}
	return resolver, r.pkiResolve(resolver), nil
}

func (r *Service) GetResolver(party view.Identity) (driver.Resolver, error) {
	return r.resolver(party)
}

func (r *Service) resolver(party view.Identity) (*Resolver, error) {
	// We can skip this check, but in case the long term was passed directly, this is going to spare us a DB lookup
	resolver, err := r.rootEndpoint(party)
	if err == nil {
		return resolver, nil
	}
	logger.Debugf("resolving via binding for %s", party)
	party, err = r.bindingKVS.GetLongTerm(party)
	if err != nil {
		return nil, err
	}
	logger.Debugf("continue to [%s]", party)
	resolver, err = r.rootEndpoint(party)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting identity for [%s]", party)
	}

	return resolver, nil
}

func (r *Service) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	if longTerm.Equal(ephemeral) {
		logger.Debugf("cannot bind [%s] to [%s], they are the same", longTerm, ephemeral)
		return nil
	}

	logger.Debugf("bind [%s] to [%s]", ephemeral, longTerm)

	if err := r.bindingKVS.PutBinding(ephemeral, longTerm); err != nil {
		return errors.WithMessagef(err, "failed storing binding of [%s]  to [%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	}

	return nil
}

func (r *Service) IsBoundTo(a view.Identity, b view.Identity) bool {
	ok, err := r.bindingKVS.HaveSameBinding(a, b)
	if err != nil {
		logger.Errorf("error fetching entries [%s] and [%s]: %v", a, b, err)
	}
	return ok
}

func (r *Service) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// search in the resolver list
	for _, resolver := range r.resolvers {
		if r.matchesResolver(endpoint, pkID, resolver) {
			return resolver.GetIdentity()
		}
	}
	return nil, errors.Errorf("identity not found at [%s,%s]", endpoint, view.Identity(pkID))
}

func (r *Service) matchesResolver(endpoint string, pkID []byte, resolver *Resolver) bool {
	if len(endpoint) > 0 && (endpoint == resolver.Name ||
		endpoint == resolver.Name+"."+resolver.Domain ||
		collections.ContainsValue(resolver.Addresses, endpoint) ||
		slices.Contains(resolver.Aliases, endpoint)) {
		return true
	}

	return len(pkID) > 0 && (bytes.Equal(pkID, resolver.Id) ||
		bytes.Equal(pkID, r.pkiResolve(resolver)))
}

func (r *Service) AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	logger.Debugf("adding resolver [%s,%s,%v,%v,%s]", name, domain, addresses, aliases, view.Identity(id))

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
	resolver.PKILock.RLock()
	if len(resolver.PKI) != 0 {
		resolver.PKILock.RUnlock()
		return resolver.PKI
	}
	resolver.PKILock.RUnlock()

	resolver.PKILock.Lock()
	defer resolver.PKILock.Unlock()
	if len(resolver.PKI) == 0 {
		resolver.PKI = r.ExtractPKI(resolver.Id)
	}
	return resolver.PKI
}

func (r *Service) ExtractPKI(id []byte) []byte {
	r.pkiExtractorsLock.RLock()
	defer r.pkiExtractorsLock.RUnlock()

	for _, extractor := range r.publicKeyExtractors {
		if pk, err := extractor.ExtractPublicKey(id); pk != nil {
			logger.Debugf("pki resolved for [%s]", id)
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

func (r *Service) rootEndpoint(party view.Identity) (*Resolver, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	for _, resolver := range r.resolvers {
		logger.Debugf("Compare [%s] [%s]", party.UniqueID(), view.Identity(resolver.Id).UniqueID())
		if bytes.Equal(resolver.Id, party) {
			return resolver, nil
		}
	}

	return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
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
