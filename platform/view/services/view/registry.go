/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"
	"regexp"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ResponderAuthorizer decides whether a remote node may start a responder.
type ResponderAuthorizer interface {
	// Authorize returns nil if caller may start the responder. caller is the FSC node identity
	// the transport authenticated, resolved from the first message's sender. A refusal returns
	// an error wrapping ErrDenied; any other error is an authorizer failure. Authorize must
	// return once ctx is done, and may be called concurrently.
	Authorize(ctx context.Context, caller view.Identity) error
}

// AuthorizerFunc adapts a function to ResponderAuthorizer.
type AuthorizerFunc func(ctx context.Context, caller view.Identity) error

// Authorize calls f(ctx, caller).
func (f AuthorizerFunc) Authorize(ctx context.Context, caller view.Identity) error {
	return f(ctx, caller)
}

// ResponderOption configures a responder registration.
type ResponderOption func(*responderEntry)

// WithAuthorizer sets the authorizer that must accept the remote node before the responder
// starts.
func WithAuthorizer(a ResponderAuthorizer) ResponderOption {
	return func(e *responderEntry) { e.authorizer = a }
}

// WithIdentity sets the identity the responder runs as. Without it, the responder runs as the
// node's default identity.
func WithIdentity(id view.Identity) ResponderOption {
	return func(e *responderEntry) { e.id = id }
}

// WithProtocol also registers the responder under the protocol name "<component>/<name>", so
// that a peer finds it by that name whatever the Go type path of its initiator view. The
// initiator view states the name by implementing ProtocolView. component and name must match
// [a-z0-9-]+. A responder registered for one name answers sessions under it from initiators
// of any type; it is still found by the type path of initiatedBy, for peers whose initiator
// declares no name.
func WithProtocol(component, name string) ResponderOption {
	return func(e *responderEntry) { e.protocol = component + "/" + name }
}

// ProtocolView is implemented by an initiator view that runs a protocol with a stable name.
// The sessions it opens with Context.GetSession carry the name "<component>/<name>", and the
// responder registered for it with WithProtocol answers them, even after the view's Go type
// moved to another package. A view it runs in a child context opens sessions under the name
// when it passes Initiator() as the caller to GetSession, as FSC's bundled views do.
type ProtocolView interface {
	view.View
	Protocol() (component, name string)
}

// protocolName is the form of a protocol name.
var protocolName = regexp.MustCompile(`^[a-z0-9-]+/[a-z0-9-]+$`)

// protocolOf returns the protocol name "<component>/<name>" v declares, or "" if it declares
// none or an invalid one.
func protocolOf(v view.View) string {
	pv, ok := v.(ProtocolView)
	if !ok {
		return ""
	}
	component, name := pv.Protocol()
	if p := component + "/" + name; protocolName.MatchString(p) {
		return p
	}
	logger.Warnf("view [%s] declares an invalid protocol name [%s/%s], ignoring it", GetIdentifier(v), component, name)
	return ""
}

// responderEntry is what an initiator ID resolves to: the responder and how it runs.
type responderEntry struct {
	view       view.View
	id         view.Identity
	authorizer ResponderAuthorizer
	// protocol is the name set with WithProtocol, empty if none.
	protocol string
}

// Registry is responsible for registering and retrieving view factories, views, and responders.
// It is safe for concurrent use.
type Registry struct {
	factoriesSync sync.RWMutex
	viewsSync     sync.RWMutex

	// responders maps an initiator view ID to the responder that answers it. One responder
	// can answer several initiators, each with its own identity and authorizer.
	responders map[string]*responderEntry
	// protocols maps a protocol name to its responder. It is a map of its own, so that a
	// protocol name never resolves to a type path that happens to look like one.
	protocols map[string]*responderEntry
	// views holds the views registered with an empty initiator, by view ID, for GetView.
	views     map[string]view.View
	factories map[string]Factory
}

// NewRegistry returns a new instance of the view registry.
func NewRegistry() *Registry {
	return &Registry{
		responders: map[string]*responderEntry{},
		protocols:  map[string]*responderEntry{},
		views:      map[string]view.View{},
		factories:  map[string]Factory{},
	}
}

// RegisterFactory registers a view factory for the given ID.
func (cm *Registry) RegisterFactory(id string, factory Factory) error {
	logger.Debugf("Register View Factory [%s,%t]", id, factory)
	cm.factoriesSync.Lock()
	defer cm.factoriesSync.Unlock()
	cm.factories[id] = factory
	return nil
}

// NewView returns a new view instance for the given ID and input.
func (cm *Registry) NewView(id string, in []byte) (f view.View, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("new view triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Wrapf(ErrViewInstantiationFailed, "panic creating view [%s]: %v", id, r)
		}
	}()

	cm.factoriesSync.RLock()
	factory, ok := cm.factories[id]
	cm.factoriesSync.RUnlock()
	if !ok {
		return nil, errors.Wrapf(ErrFactoryNotFound, "no factory found for id [%s]", id)
	}
	return factory.NewView(in)
}

// RegisterResponderFactory registers a responder view factory for the given initiator view.
func (cm *Registry) RegisterResponderFactory(factory Factory, initiatedBy any) error {
	responder, err := factory.NewView(nil)
	if err != nil {
		return err
	}
	return cm.RegisterResponder(responder, initiatedBy)
}

// RegisterResponder registers a responder view for the given initiator view.
func (cm *Registry) RegisterResponder(responder view.View, initiatedBy any) error {
	return cm.RegisterResponderWithOptions(responder, initiatedBy)
}

// RegisterResponderWithIdentity registers a responder view for the given initiator view and responder identity.
func (cm *Registry) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy any) error {
	return cm.RegisterResponderWithOptions(responder, initiatedBy, WithIdentity(id))
}

// RegisterResponderWithOptions registers responder to answer initiatedBy, a view or a view ID.
// An empty view ID registers responder as a view that GetView returns instead; such a
// registration cannot have an authorizer, since no remote node starts it.
//
// A registration for an initiator that already has a responder replaces its responder and
// identity. It replaces the authorizer only if it sets one: a later registration without
// WithAuthorizer, such as a default one made by another module, keeps the existing authorizer
// instead of silently opening the responder to every peer. The same holds for a protocol name:
// a registration with WithProtocol for a name that already has a responder replaces it, and a
// later registration for the same initiator without WithProtocol keeps the name, and one with
// another name releases the old name. A name registered for one initiator cannot be registered
// for another: that registration fails.
func (cm *Registry) RegisterResponderWithOptions(responder view.View, initiatedBy any, opts ...ResponderOption) error {
	var initiatedByID string
	switch t := initiatedBy.(type) {
	case view.View:
		initiatedByID = cm.GetIdentifier(t)
	case string:
		initiatedByID = t
	default:
		return errors.New("initiatedBy must be a view or a string")
	}
	entry := &responderEntry{view: responder}
	for _, opt := range opts {
		opt(entry)
	}

	cm.viewsSync.Lock()
	defer cm.viewsSync.Unlock()

	logger.Debugf("registering responder [%s] for initiator [%s] with identity [%s] and protocol [%s]", GetIdentifier(responder), initiatedByID, entry.id, entry.protocol)
	if entry.protocol != "" && !protocolName.MatchString(entry.protocol) {
		return errors.Errorf("invalid protocol name [%s]: want [a-z0-9-]+/[a-z0-9-]+", entry.protocol)
	}
	if len(initiatedByID) == 0 {
		if entry.authorizer != nil || entry.protocol != "" {
			return errors.New("an authorizer or protocol needs an initiator")
		}
		cm.views[GetIdentifier(responder)] = responder
		return nil
	}
	old := cm.responders[initiatedByID]
	if old != nil {
		if entry.protocol == "" {
			entry.protocol = old.protocol
		}
		if entry.authorizer == nil {
			entry.authorizer = old.authorizer
		}
	}
	// A name belongs to one initiator, so that its responder and authorizer are those of that
	// initiator's registration and no other registration can take them over.
	if prev, ok := cm.protocols[entry.protocol]; ok && prev != old {
		return errors.Errorf("protocol [%s] is registered for another initiator", entry.protocol)
	}
	if old != nil && old.protocol != entry.protocol {
		delete(cm.protocols, old.protocol)
	}
	cm.responders[initiatedByID] = entry
	if entry.protocol != "" {
		cm.protocols[entry.protocol] = entry
	}
	return nil
}

// GetResponder returns the responder view for the given initiator view.
func (cm *Registry) GetResponder(initiatedBy any) (view.View, error) {
	var initiatedByID string
	switch t := initiatedBy.(type) {
	case view.View:
		initiatedByID = cm.GetIdentifier(t)
	case string:
		initiatedByID = t
	default:
		return nil, errors.New("initiatedBy must be a view or a string")
	}
	r, _, _, err := cm.ResponderForCaller("", initiatedByID)
	return r, err
}

// GetIdentifier returns the identifier for the given view.
func (*Registry) GetIdentifier(f view.View) string {
	return GetIdentifier(f)
}

// GetView returns the view registered with an empty initiator under the given ID.
func (cm *Registry) GetView(id string) (view.View, error) {
	cm.viewsSync.RLock()
	v, ok := cm.views[id]
	cm.viewsSync.RUnlock()
	if !ok {
		return nil, errors.Wrapf(ErrViewNotFound, "initiator not found for [%s]", id)
	}
	return v, nil
}

// ExistResponderForCaller returns the responder view and identity for the given caller identifier.
func (cm *Registry) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	r, id, _, err := cm.ResponderForCaller("", caller)
	return r, id, err
}

// ResponderForCaller returns the responder registered for protocol, a protocol name, or, if
// protocol is empty or has no responder, for the initiator view ID caller. It also returns the
// identity the responder runs as (none for the default identity) and its authorizer (nil if any
// peer may start it). It returns an error wrapping ErrResponderNotFound if neither has one.
func (cm *Registry) ResponderForCaller(protocol, caller string) (view.View, view.Identity, ResponderAuthorizer, error) {
	cm.viewsSync.RLock()
	defer cm.viewsSync.RUnlock()

	e, ok := cm.protocols[protocol]
	if !ok {
		e, ok = cm.responders[caller]
	}
	if !ok {
		return nil, nil, nil, errors.Wrapf(ErrResponderNotFound, "no view found initiatable by [%s]", caller)
	}
	return e.view, e.id, e.authorizer, nil
}

var (
	identifierCache sync.Map
	nameCache       sync.Map
)

// GetIdentifier returns the identifier for the given view.
func GetIdentifier(f view.View) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	if v, ok := identifierCache.Load(t); ok {
		id, ok := v.(string)
		if !ok {
			panic(errors.Errorf("unexpected cached identifier type [%T]", v))
		}
		return id
	}

	ptr := t
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	id := t.PkgPath() + "/" + t.Name()
	identifierCache.Store(ptr, id)
	return id
}

// GetName returns the name for the given view.
func GetName(f view.View) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	if v, ok := nameCache.Load(t); ok {
		name, ok := v.(string)
		if !ok {
			panic(errors.Errorf("unexpected cached name type [%T]", v))
		}
		return name
	}

	ptr := t
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := t.Name()
	nameCache.Store(ptr, name)
	return name
}

// GetRegistry returns an instance of the view registry from the service provider.
// It panics if no instance is found.
func GetRegistry(sp services.Provider) *Registry {
	s, err := sp.GetService(reflect.TypeFor[*Registry]())
	if err != nil {
		panic(err)
	}
	r, ok := s.(*Registry)
	if !ok {
		panic(errors.Errorf("unexpected service type [%T] for view registry", s))
	}
	return r
}
