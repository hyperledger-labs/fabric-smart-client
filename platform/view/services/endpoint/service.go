/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"context"
	"maps"
	"reflect"
	"slices"
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

// ErrNotFound signals that a resolver was not found
var ErrNotFound = errors.New("not found")

// PublicKeyExtractor extracts public keys from identities for PKI operations.
//
//go:generate counterfeiter -o mock/key_extractor.go -fake-name PublicKeyExtractor . PublicKeyExtractor
type PublicKeyExtractor interface {
	// ExtractPublicKey returns the public key corresponding to the passed identity.
	// Returns nil and an error if the public key cannot be extracted.
	ExtractPublicKey(id view.Identity) (any, error)
}

// PublicKeyIDSynthesizer generates unique identifiers from public keys.
//
//go:generate counterfeiter -o mock/id_synth.go -fake-name PublicKeyIDSynthesizer . PublicKeyIDSynthesizer
type PublicKeyIDSynthesizer interface {
	// PublicKeyID generates a unique identifier for the given public key.
	// Returns an error if the key type is unsupported or cannot be processed.
	PublicKeyID(any) ([]byte, error)
}

// ResolverInfo carries information about a resolver including its identity,
// network addresses, and aliases.
type ResolverInfo struct {
	ID        []byte
	Name      string
	Domain    string
	Addresses map[PortName]string
	Aliases   []string
	PKI       []byte
}

// Resolver wraps ResolverInfo with additional management fields including
// a lock for thread-safe PKI operations.
//
// Once a Resolver is registered with a Service, the Service never modifies its
// ResolverInfo fields other than PKI, which PKILock guards. UpdateResolver registers an
// updated copy instead, so the fields of a Resolver returned by GetResolver or Resolver,
// other than PKI, can be read without locking but do not reflect later updates. Read PKI
// through Service.PkiResolve.
type Resolver struct {
	ResolverInfo
	PKILock sync.RWMutex
}

// GetName returns the name of the resolver.
func (r *Resolver) GetName() string { return r.Name }

// GetId returns the identity associated with this resolver.
//
//nolint:revive // var-naming: renaming this exported method is an API break; see follow-up
func (r *Resolver) GetId() view.Identity { return r.ID }

// GetAddress returns the address for the specified port name.
func (r *Resolver) GetAddress(port PortName) string { return r.Addresses[port] }

// GetAddresses returns all addresses mapped by port name.
func (r *Resolver) GetAddresses() map[PortName]string { return r.Addresses }

// GetIdentity returns the identity associated with this resolver.
func (r *Resolver) GetIdentity() (view.Identity, error) {
	return r.ID, nil
}

// BindingStore is an alias for cdriver.BindingStore
//
//go:generate counterfeiter -o mock/binding_store.go -fake-name BindingStore . BindingStore
type BindingStore = cdriver.BindingStore

// Service manages endpoint resolution, identity binding, and PKI extraction
// for the view service layer.
type Service struct {
	resolvers []*Resolver
	// resolversMap indexes resolvers by name, name.domain, aliases, addresses, ID and
	// PKI ID. It never holds the empty key, so an empty label, identity or name matches
	// no resolver.
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
	s, err := sp.GetService(reflect.TypeFor[*Service]())
	if err != nil {
		panic(err)
	}
	svc, ok := s.(*Service)
	if !ok {
		panic(errors.Errorf("unexpected service type [%T] for endpoint service", s))
	}
	return svc
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
	maps.Copy(out, resolver.GetAddresses())
	return resolver.GetId(), out, raw, nil
}

// GetResolver returns the resolver associated with the given identity.
func (r *Service) GetResolver(ctx context.Context, id view.Identity) (*Resolver, error) {
	return r.lookupResolver(ctx, id)
}

// Bind associates ephemeral identities with a long-term identity in the binding store.
// Identities that are equal to the longTerm identity are filtered out.
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
		return errors.WithMessage(err, "failed storing bindings")
	}
	return nil
}

// IsBoundTo checks if two identities are bound to the same long-term identity.
func (r *Service) IsBoundTo(ctx context.Context, a, b view.Identity) bool {
	ok, err := r.bindingKVS.HaveSameBinding(ctx, a, b)
	if err != nil {
		logger.Errorf("error fetching entries [%s] and [%s]: %v", a, b, err)
	}
	return ok
}

// GetIdentity returns the identity associated with the given public key ID or label.
// A non-empty pkID is authoritative: the label is looked up only when pkID is empty, since
// a label is matched against names, aliases and addresses, none of which a remote peer
// proves. An empty label is not looked up.
func (r *Service) GetIdentity(label string, pkID []byte) (view.Identity, error) {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	key := label
	if len(pkID) != 0 {
		key = string(pkID)
	}
	if resolver, ok := r.resolversMap[key]; ok && len(key) != 0 {
		return resolver.GetIdentity()
	}

	return nil, errors.Wrapf(ErrNotFound, "identity not found at [%s,%s]", label, view.Identity(pkID))
}

// UpdateResolver looks up the resolver that name matches in the index (by name,
// name.domain, alias, address, ID or PKI ID), replaces its addresses with the given ones,
// adds the given aliases it does not have yet, and returns its ID. domain and id are
// ignored for an existing resolver.
//
// The resolver is replaced by an updated copy at the same position in Resolvers. The copy
// is indexed like a newly added resolver, taking over its keys from any other resolver
// that holds them; keys of the old resolver that the copy no longer has, such as replaced
// addresses, are removed unless another resolver holds them. Resolvers obtained before
// the update keep the old addresses.
//
// If name matches no resolver, UpdateResolver behaves like AddResolver. An empty name
// matches no resolver. UpdateResolver returns an error, and changes nothing, if an address
// key is not a port name accepted by AddResolver.
func (r *Service) UpdateResolver(
	name string,
	domain string,
	addresses map[string]string,
	aliases []string,
	id []byte,
) (view.Identity, error) {
	logger.Debugf("updating resolver [%s,%s,%v,%v,%s]", name, domain, addresses, aliases, view.Identity(id))

	ports, err := convert(addresses)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed updating resolver [%s]", name)
	}

	r.resolversMutex.Lock()
	old, ok := r.resolversMap[name]
	if ok {
		old.PKILock.RLock()
		updated := &Resolver{ResolverInfo: old.ResolverInfo}
		old.PKILock.RUnlock()
		updated.Addresses = ports
		updated.Aliases = slices.Clone(old.Aliases)
		for _, alias := range aliases {
			if len(alias) != 0 && !slices.Contains(updated.Aliases, alias) {
				updated.Aliases = append(updated.Aliases, alias)
			}
		}

		r.unindexResolver(old)
		// every resolver in resolversMap is also in r.resolvers, since appendResolver,
		// RemoveResolver and UpdateResolver change both together under the write lock, so
		// the index is never -1
		r.resolvers[slices.Index(r.resolvers, old)] = updated
		r.indexResolver(updated, updated.PKI)
		r.resolversMutex.Unlock()
		return updated.ID, nil
	}
	r.resolversMutex.Unlock()

	return r.AddResolver(name, domain, addresses, aliases, id)
}

// AddResolver registers a resolver and returns nil. If name already matches a resolver in
// the index (by name, name.domain, alias, address, ID or PKI ID), AddResolver binds id to
// that resolver's identity and returns it instead; the other arguments are only validated.
// Node configurations rely on this: replicas of a node share its name, and a Fabric
// resolver binds a node's MSP identity to its FSC identity by reusing the node's name.
//
// An empty name matches no resolver, so each resolver without a name is registered
// separately and is reachable only by its aliases, addresses, ID and PKI ID. The config
// loaders reject entries without a name, so only programmatic callers register such
// resolvers.
//
// Each address key must name a PortName (Listen, View or P2P), compared case-insensitively,
// at most once; AddResolver returns an error, and registers nothing, for any other key.
func (r *Service) AddResolver(
	name string,
	domain string,
	addresses map[string]string,
	aliases []string,
	id []byte,
) (view.Identity, error) {
	logger.Debugf("adding resolver [%s,%s,%v,%v,%s]", name, domain, addresses, aliases, view.Identity(id))

	ports, err := convert(addresses)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed adding resolver [%s]", name)
	}

	// is there a resolver with the same name or clashing aliases?
	r.resolversMutex.RLock()
	resolver, ok := r.resolversMap[name]
	if ok {
		// Then bind
		r.resolversMutex.RUnlock()
		return resolver.ID, r.Bind(context.Background(), resolver.ID, id)
	}
	for _, alias := range aliases {
		if existing, ok := r.resolversMap[alias]; ok {
			logger.Warnf("alias [%s] already defined by resolver [%s]", alias, existing.Name)
		}
	}
	r.resolversMutex.RUnlock()

	newResolver := &Resolver{
		Name:      name,
		Domain:    domain,
		Addresses: ports,
		Aliases:   aliases,
		ID:        id,
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

	r.unindexResolver(resolver)

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

// AddPublicKeyExtractor adds a public key extractor to the service.
// Returns an error if the extractor is nil.
func (r *Service) AddPublicKeyExtractor(publicKeyExtractor PublicKeyExtractor) error {
	r.pkiExtractorsLock.Lock()
	defer r.pkiExtractorsLock.Unlock()

	if publicKeyExtractor == nil {
		return errors.New("pki resolver should not be nil")
	}
	r.publicKeyExtractors = append(r.publicKeyExtractors, publicKeyExtractor)
	return nil
}

// SetPublicKeyIDSynthesizer sets the synthesizer used to generate public key IDs.
func (r *Service) SetPublicKeyIDSynthesizer(publicKeyIDSynthesizer PublicKeyIDSynthesizer) {
	r.publicKeyIDSynthesizer = publicKeyIDSynthesizer
}

// ExtractPKI extracts and synthesizes a public key ID from the given identity.
// It tries each registered extractor in order until one succeeds.
// Returns nil if no extractor can extract a public key.
func (r *Service) ExtractPKI(id []byte) []byte {
	r.pkiExtractorsLock.RLock()
	defer r.pkiExtractorsLock.RUnlock()

	for _, extractor := range r.publicKeyExtractors {
		pk, err := extractor.ExtractPublicKey(id)
		if pk == nil {
			logger.Debugf("pki not resolved by [%s] for [%s]: [%s]", logging.Identifier(extractor), id, err)
			continue
		}
		logger.Debugf("pki resolved for [%s]", id)
		pkiID, err := r.publicKeyIDSynthesizer.PublicKeyID(pk)
		if err != nil {
			logger.Errorf("failed to synthesize public key ID for [%s]: %v", id, err)
			continue
		}
		return pkiID
	}
	logger.Warnf("cannot resolve pki for [%s]", id)
	return nil
}

// ResolveIdentities resolves a list of endpoint strings to their corresponding identities.
// Returns an error if any endpoint cannot be resolved.
func (r *Service) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	var ids []view.Identity
	for _, endpoint := range endpoints {
		id, err := r.GetIdentity(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the identity at %s", endpoint)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// Resolver returns the resolver and PKI ID for the given identity.
func (r *Service) Resolver(ctx context.Context, id view.Identity) (*Resolver, []byte, error) {
	resolver, err := r.lookupResolver(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return resolver, r.PkiResolve(resolver), nil
}

// Resolvers returns a copy of all registered resolver information.
func (r *Service) Resolvers() []ResolverInfo {
	r.resolversMutex.RLock()
	defer r.resolversMutex.RUnlock()

	// PKI is written under PKILock, not resolversMutex. Copy each resolver while
	// holding that lock so the copy does not race with PkiResolve. The lock order
	// is resolversMutex then PKILock.
	var res []ResolverInfo
	for _, resolver := range r.resolvers {
		resolver.PKILock.RLock()
		res = append(res, resolver.ResolverInfo)
		resolver.PKILock.RUnlock()
	}
	return res
}

// appendResolver adds a resolver to the internal list and indexes it.
// The caller must hold resolversMutex for writing.
func (r *Service) appendResolver(newResolver *Resolver, pkiID []byte) {
	r.resolvers = append(r.resolvers, newResolver)
	r.indexResolver(newResolver, pkiID)
}

// indexResolver indexes a resolver in resolversMap by name, name.domain, aliases,
// addresses, ID and PKI ID, overwriting the entries of any other resolver under the same
// keys. Empty keys are skipped, and a resolver without a name is indexed by neither name
// nor name.domain. The caller must hold resolversMutex for writing.
func (r *Service) indexResolver(newResolver *Resolver, pkiID []byte) {
	if len(newResolver.Name) != 0 {
		// by name
		r.resolversMap[newResolver.Name] = newResolver
		// by name + domain
		r.resolversMap[newResolver.Name+"."+newResolver.Domain] = newResolver
	}
	// by alias
	for _, alias := range newResolver.Aliases {
		if len(alias) != 0 {
			r.resolversMap[alias] = newResolver
		}
	}

	// by addresses
	for _, address := range newResolver.Addresses {
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

// unindexResolver deletes every resolversMap entry that points to resolver.
// The caller must hold resolversMutex for writing.
func (r *Service) unindexResolver(resolver *Resolver) {
	maps.DeleteFunc(r.resolversMap, func(_ string, value *Resolver) bool { return value == resolver })
}

// PkiResolve extracts and caches the PKI ID for a resolver.
// It uses double-checked locking to ensure thread-safe lazy initialization.
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

// resolver attempts to find a resolver for the given identity, checking both
// direct lookups and binding store lookups for long-term identities.
func (r *Service) lookupResolver(ctx context.Context, party view.Identity) (*Resolver, error) {
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

// resolverByIdentity looks up a resolver directly by identity without checking bindings.
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

// portNameMap maps lowercase port name strings to their PortName constants.
var portNameMap = map[string]PortName{
	strings.ToLower(string(ListenPort)): ListenPort,
	strings.ToLower(string(ViewPort)):   ViewPort,
	strings.ToLower(string(P2PPort)):    P2PPort,
}

// convert maps each key of o to its PortName, compared case-insensitively. It returns an
// error for a key that names no PortName or the same PortName as another key, so that a
// misspelled or repeated key fails instead of dropping an address.
func convert(o map[string]string) (map[PortName]string, error) {
	r := map[PortName]string{}
	for k, v := range o {
		port, ok := portNameMap[strings.ToLower(k)]
		if !ok {
			return nil, errors.Errorf("unknown address key [%s], expected one of [%s, %s, %s]", k, ListenPort, ViewPort, P2PPort)
		}
		if _, dup := r[port]; dup {
			return nil, errors.Errorf("duplicate address key [%s]", k)
		}
		r[port] = v
	}
	return r, nil
}
