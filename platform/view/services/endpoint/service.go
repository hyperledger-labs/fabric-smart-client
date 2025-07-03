/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"context"
	"net"
	"reflect"
	"strings"
	"sync"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

var logger = logging.MustGetLogger()

// PortName is the type variable for the socket ports
type PortName string

const (
	// ListenPort is the port at which the FSC node might listen for some service
	ListenPort PortName = "Listen"
	// ViewPort is the port on which the View Service Server respond
	ViewPort PortName = "View"
	// P2PPort is the port on which the P2P Communication Layer respond
	P2PPort PortName = "P2P"
)

// PublicKeyExtractor extracts public keys from identities
type PublicKeyExtractor interface {
	// ExtractPublicKey returns the public key corresponding to the passed identity
	ExtractPublicKey(id view.Identity) (any, error)
}

type PublicKeyIDSynthesizer interface {
	PublicKeyID(any) []byte
}

type Resolver struct {
	Name           string
	Domain         string
	Addresses      map[PortName]string
	Aliases        []string
	PKI            []byte
	PKILock        sync.RWMutex
	ID             []byte
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *Resolver) GetName() string { return r.Name }

func (r *Resolver) GetId() view.Identity { return r.ID }

func (r *Resolver) GetAddress(port PortName) string { return r.Addresses[port] }

func (r *Resolver) GetAddresses() map[PortName]string { return r.Addresses }

func (r *Resolver) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		id, _, err := r.IdentityGetter()
		return id, err
	}
	return r.ID, nil
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
	resolvers      []Resolver
	resolversMap   map[string]*Resolver
	resolversMutex sync.RWMutex
	bindingKVS     driver2.BindingStore

	pkiExtractorsLock      sync.RWMutex
	publicKeyExtractors    []PublicKeyExtractor
	publicKeyIDSynthesizer PublicKeyIDSynthesizer
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(bindingKVS driver2.BindingStore) (*Service, error) {
	er := &Service{
		bindingKVS:             bindingKVS,
		publicKeyExtractors:    []PublicKeyExtractor{},
		publicKeyIDSynthesizer: DefaultPublicKeyIDSynthesizer{},
		resolvers:              []Resolver{},
		resolversMap:           map[string]*Resolver{},
	}
	return er, nil
}

// GetService returns an instance of the endpoint service.
// It panics, if no instance is found.
func GetService(sp services.Provider) *Service {
	s, err := sp.GetService(reflect.TypeOf((*Service)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(*Service)
}

// Resolve returns the endpoints of the passed identity.
// If the passed identity does not have any endpoint set, the service checks
// if the passed identity is bound to another identity that is returned together with its endpoints and public-key identifier.
func (r *Service) Resolve(ctx context.Context, party view.Identity) (view.Identity, map[PortName]string, []byte, error) {
	resolver, raw, err := r.Resolver(ctx, party)
	if err != nil {
		return nil, nil, nil, err
	}
	if resolver == nil {
		return nil, nil, raw, nil
	}
	logger.Debugf("resolved [%s] to [%s] with ports [%v]", party, resolver.GetId(), resolver.GetAddresses())
	out := map[PortName]string{}
	for name, s := range resolver.GetAddresses() {
		out[name] = s
	}
	return resolver.GetId(), out, raw, nil
}

func (r *Service) GetResolver(ctx context.Context, party view.Identity) (*Resolver, error) {
	return r.resolver(ctx, party)
}

func (r *Service) Bind(ctx context.Context, longTerm view.Identity, ephemeral view.Identity) error {
	if longTerm.Equal(ephemeral) {
		logger.Debugf("cannot bind [%s] to [%s], they are the same", longTerm, ephemeral)
		return nil
	}

	logger.Debugf("bind [%s] to [%s]", ephemeral, longTerm)

	if err := r.bindingKVS.PutBinding(ctx, ephemeral, longTerm); err != nil {
		return errors.WithMessagef(err, "failed storing binding of [%s]  to [%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	}

	return nil
}

func (r *Service) IsBoundTo(ctx context.Context, a view.Identity, b view.Identity) bool {
	ok, err := r.bindingKVS.HaveSameBinding(ctx, a, b)
	if err != nil {
		logger.Errorf("error fetching entries [%s] and [%s]: %v", a, b, err)
	}
	return ok
}

func (r *Service) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// by endpoint
	resolver, ok := r.resolversMap[endpoint]
	if ok {
		return resolver.GetIdentity()
	}

	// by pkID
	resolver, ok = r.resolversMap[string(pkID)]
	if ok {
		return resolver.GetIdentity()
	}

	return nil, errors.Errorf("identity not found at [%s,%s]", endpoint, view.Identity(pkID))
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
			return resolver.ID, r.Bind(context.Background(), resolver.ID, id)
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
	newResolver := &Resolver{
		Name:      name,
		Domain:    domain,
		Addresses: convert(addresses),
		Aliases:   aliases,
		ID:        id,
	}
	pkiID := r.pkiResolve(newResolver)

	r.appendResolver(newResolver, pkiID)

	return nil, nil
}

func (r *Service) AddPublicKeyExtractor(publicKeyExtractor PublicKeyExtractor) error {
	r.pkiExtractorsLock.Lock()
	defer r.pkiExtractorsLock.Unlock()

	if publicKeyExtractor == nil {
		return errors.New("pki resolver should not be nil")
	}
	r.publicKeyExtractors = append(r.publicKeyExtractors, publicKeyExtractor)
	return nil
}

func (r *Service) SetPublicKeyIDSynthesizer(publicKeyIDSynthesizer PublicKeyIDSynthesizer) {
	r.publicKeyIDSynthesizer = publicKeyIDSynthesizer
}

func (r *Service) ExtractPKI(id []byte) []byte {
	r.pkiExtractorsLock.RLock()
	defer r.pkiExtractorsLock.RUnlock()

	for _, extractor := range r.publicKeyExtractors {
		if pk, err := extractor.ExtractPublicKey(id); pk != nil {
			logger.Debugf("pki resolved for [%s]", id)
			return r.publicKeyIDSynthesizer.PublicKeyID(pk)
		} else {
			logger.Debugf("pki not resolved by [%s] for [%s]: [%s]", logging.Identifier(extractor), id, err)
		}
	}
	logger.Warnf("cannot resolve pki for [%s]", id)
	return nil
}

func (r *Service) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	var ids []view.Identity
	for _, endpoint := range endpoints {
		id, err := r.GetIdentity(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the idnetity at %s", endpoint)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func (r *Service) Resolver(ctx context.Context, party view.Identity) (*Resolver, []byte, error) {
	resolver, err := r.resolver(ctx, party)
	if err != nil {
		return nil, nil, err
	}
	return resolver, r.pkiResolve(resolver), nil
}

func (r *Service) Resolvers() []Resolver {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// clone r.resolvers
	var res []Resolver
	for _, resolver := range r.resolvers {
		res = append(res, resolver)
	}
	return res
}

func (r *Service) appendResolver(newResolver *Resolver, pkiID []byte) {
	r.resolvers = append(r.resolvers, *newResolver)

	// by name
	r.resolversMap[newResolver.Name] = newResolver
	// by alias
	for _, alias := range newResolver.Aliases {
		r.resolversMap[alias] = newResolver
	}
	// by name + domain
	r.resolversMap[newResolver.Name+"."+newResolver.Domain] = newResolver

	// by addresses
	for _, address := range newResolver.Addresses {
		r.resolversMap[address] = newResolver
	}

	// by id
	r.resolversMap[string(newResolver.ID)] = newResolver

	// by pkiResolver
	r.resolversMap[string(pkiID)] = newResolver

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
		resolver.PKI = r.ExtractPKI(resolver.ID)
	}
	return resolver.PKI
}

func (r *Service) resolver(ctx context.Context, party view.Identity) (*Resolver, error) {
	// We can skip this check, but in case the long term was passed directly, this is going to spare us a DB lookup
	resolver, err := r.resolverByIdentity(party)
	if err == nil {
		return resolver, nil
	}
	logger.Debugf("resolving via binding for %s", party)
	party, err = r.bindingKVS.GetLongTerm(ctx, party)
	if err != nil {
		return nil, err
	}
	logger.Debugf("continue to [%s]", party)
	resolver, err = r.resolverByIdentity(party)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting identity for [%s]", party)
	}

	return resolver, nil
}

func (r *Service) resolverByIdentity(party view.Identity) (*Resolver, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// by identity
	resolver, ok := r.resolversMap[string(party)]
	if ok {
		return resolver, nil
	}

	return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
}

var portNameMap = map[string]PortName{
	strings.ToLower(string(ListenPort)): ListenPort,
	strings.ToLower(string(ViewPort)):   ViewPort,
	strings.ToLower(string(P2PPort)):    P2PPort,
}

func convert(o map[string]string) map[PortName]string {
	r := map[PortName]string{}
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
