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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

var (
	// ErrNotFound signals that a resolver was not found
	ErrNotFound = errors.New("not found")
)

// PublicKeyExtractor extracts public keys from identities
type PublicKeyExtractor interface {
	// ExtractPublicKey returns the public key corresponding to the passed identity
	ExtractPublicKey(id view.Identity) (any, error)
}

type PublicKeyIDSynthesizer interface {
	PublicKeyID(any) []byte
}

// ResolverInfo carries information about a resolver
type ResolverInfo struct {
	ID          []byte
	Name        string
	Domain      string
	Addresses   map[PortName]string
	AddressList []string
	Aliases     []string
	PKI         []byte
}

// Resolver wraps ResolverInfo with additional management fields
type Resolver struct {
	ResolverInfo
	PKILock sync.RWMutex
}

func (r *Resolver) GetName() string { return r.Name }

func (r *Resolver) GetId() view.Identity { return r.ID }

func (r *Resolver) GetAddress(port PortName) string { return r.Addresses[port] }

func (r *Resolver) GetAddresses() map[PortName]string { return r.Addresses }

func (r *Resolver) GetIdentity() (view.Identity, error) {
	return r.ID, nil
}

// NetworkMember is a peer's representation
type NetworkMember struct {
	Endpoint         string
	Metadata         []byte
	PKIid            []byte
	InternalEndpoint string
}

//go:generate counterfeiter -o mock/binding_store.go -fake-name BindingStore . BindingStore

type BindingStore = cdriver.BindingStore

type Service struct {
	resolvers      []*Resolver
	resolversMap   map[string]*Resolver
	resolversMutex sync.RWMutex
	bindingKVS     cdriver.BindingStore

	pkiExtractorsLock      sync.RWMutex
	publicKeyExtractors    []PublicKeyExtractor
	publicKeyIDSynthesizer PublicKeyIDSynthesizer
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(bindingKVS BindingStore) (*Service, error) {
	er := &Service{
		bindingKVS:             bindingKVS,
		publicKeyExtractors:    []PublicKeyExtractor{},
		publicKeyIDSynthesizer: DefaultPublicKeyIDSynthesizer{},
		resolvers:              []*Resolver{},
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
func (r *Service) Resolve(ctx context.Context, id view.Identity) (view.Identity, map[PortName]string, []byte, error) {
	resolver, raw, err := r.Resolver(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if resolver == nil {
		return nil, nil, raw, nil
	}
	logger.DebugfContext(ctx, "resolved [%s] to [%s] with ports [%v]", id, resolver.GetId(), resolver.GetAddresses())
	out := map[PortName]string{}
	for name, s := range resolver.GetAddresses() {
		out[name] = s
	}
	return resolver.GetId(), out, raw, nil
}

func (r *Service) GetResolver(ctx context.Context, id view.Identity) (*Resolver, error) {
	return r.resolver(ctx, id)
}

func (r *Service) Bind(ctx context.Context, longTerm view.Identity, ephemeralIDs ...view.Identity) error {
	// filter out any identities equal to the longTerm identity
	var toBind []view.Identity
	for _, id := range ephemeralIDs {
		if !longTerm.Equal(id) {
			toBind = append(toBind, id)
		}
	}
	if len(toBind) == 0 {
		return nil
	}
	if err := r.bindingKVS.PutBindings(ctx, longTerm, toBind...); err != nil {
		return errors.WithMessagef(err, "failed storing bindings")
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

func (r *Service) GetIdentity(label string, pkID []byte) (view.Identity, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	resolver, ok := r.resolversMap[label]
	if ok {
		return resolver.GetIdentity()
	}

	// by pkID
	resolver, ok = r.resolversMap[string(pkID)]
	if ok {
		return resolver.GetIdentity()
	}

	return nil, errors.Wrapf(ErrNotFound, "identity not found at [%s,%s]", label, view.Identity(pkID))
}

// AddResolver adds a new resolver.
func (r *Service) AddResolver(
	name string,
	domain string,
	addresses map[string]string,
	aliases []string,
	id []byte,
) (view.Identity, error) {
	logger.Debugf("adding resolver [%s,%s,%v,%v,%s]", name, domain, addresses, aliases, view.Identity(id))

	// is there a resolver with the same name or clashing aliases?
	r.resolversMutex.RLock()
	resolver, ok := r.resolversMap[name]
	if ok {
		// Then bind
		r.resolversMutex.RUnlock()
		return resolver.ID, r.Bind(context.Background(), resolver.ID, id)
	}
	for _, alias := range aliases {
		_, ok := r.resolversMap[name]
		if ok {
			logger.Warnf("alias [%s] already defined by resolver [%s]", alias, resolver.Name)
		}
	}
	r.resolversMutex.RUnlock()

	// resolve addresses to their IPs, if needed
	addressList := make([]string, 0, len(addresses))
	for k, v := range addresses {
		addresses[k] = LookupIPv4(v)
		addressList = append(addressList, v)
	}

	newResolver := &Resolver{
		ResolverInfo: ResolverInfo{
			Name:        name,
			Domain:      domain,
			Addresses:   convert(addresses),
			AddressList: addressList,
			Aliases:     aliases,
			ID:          id,
		},
	}
	pkiID := r.PkiResolve(newResolver)

	r.resolversMutex.Lock()
	defer r.resolversMutex.Unlock()

	// check again
	resolver, ok = r.resolversMap[name]
	if ok {
		// Then bind
		return resolver.ID, r.Bind(context.Background(), resolver.ID, id)
	}

	r.appendResolver(newResolver, pkiID)
	return nil, nil
}

// RemoveResolver first check if a resolver is bound to the passed identity.
// If yes, the function removes the resolver.
// If no, the function does nothing.
func (r *Service) RemoveResolver(id view.Identity) (bool, error) {
	r.resolversMutex.RLock()
	_, ok := r.resolversMap[string(id)]
	r.resolversMutex.RUnlock()
	if !ok {
		return false, errors.Wrapf(ErrNotFound, "cannot find resolver for [%s]", id)
	}

	r.resolversMutex.Lock()
	defer r.resolversMutex.Unlock()

	// check again if the resolver is still there
	resolver, ok := r.resolversMap[string(id)]
	if !ok {
		return false, errors.Wrapf(ErrNotFound, "cannot find resolver for [%s]", id)
	}

	// remove from the map
	for key, value := range r.resolversMap {
		if value == resolver {
			delete(r.resolversMap, key)
		}
	}

	// remove from the resolver list
	for i, value := range r.resolvers {
		if value == resolver {
			r.resolvers[i] = r.resolvers[len(r.resolvers)-1]
			r.resolvers = r.resolvers[:len(r.resolvers)-1]
			break
		}
	}

	return true, nil
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

func (r *Service) Resolver(ctx context.Context, id view.Identity) (*Resolver, []byte, error) {
	resolver, err := r.resolver(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return resolver, r.PkiResolve(resolver), nil
}

func (r *Service) Resolvers() []ResolverInfo {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// clone r.resolvers
	var res []ResolverInfo
	for _, resolver := range r.resolvers {
		res = append(res, resolver.ResolverInfo)
	}
	return res
}

func (r *Service) appendResolver(newResolver *Resolver, pkiID []byte) {
	r.resolvers = append(r.resolvers, newResolver)

	// by name
	r.resolversMap[newResolver.Name] = newResolver
	// by alias
	for _, alias := range newResolver.Aliases {
		if len(alias) != 0 {
			r.resolversMap[alias] = newResolver
		}
	}
	// by name + domain
	r.resolversMap[newResolver.Name+"."+newResolver.Domain] = newResolver

	// by addresses
	for _, address := range newResolver.Addresses {
		if len(address) != 0 {
			r.resolversMap[address] = newResolver
		}
	}

	// by addresses
	for _, address := range newResolver.AddressList {
		if len(address) != 0 {
			r.resolversMap[address] = newResolver
		}
	}

	// by id
	if len(newResolver.ID) != 0 {
		r.resolversMap[string(newResolver.ID)] = newResolver
	}

	// by pkiResolver
	if len(pkiID) != 0 {
		r.resolversMap[string(pkiID)] = newResolver
	}
}

func (r *Service) PkiResolve(resolver *Resolver) []byte {
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
	logger.DebugfContext(ctx, "resolving via binding for %s", party)
	party, err = r.bindingKVS.GetLongTerm(ctx, party)
	if err != nil {
		return nil, err
	}
	logger.DebugfContext(ctx, "continue to [%s]", party)
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

	return nil, errors.Wrapf(ErrNotFound, "endpoint not found for identity %s", party.UniqueID())
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
