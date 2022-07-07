/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"context"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("view-sdk.manager")

type viewEntry struct {
	View      view.View
	ID        view.Identity
	Initiator bool
}

type manager struct {
	sp driver.ServiceProvider

	ctx context.Context

	factoriesSync sync.RWMutex
	viewsSync     sync.RWMutex
	contextsSync  sync.RWMutex

	contexts   map[string]disposableContext
	views      map[string][]*viewEntry
	initiators map[string]string
	factories  map[string]driver.Factory
}

func New(serviceProvider driver.ServiceProvider) *manager {
	return &manager{
		sp: serviceProvider,

		contexts:   map[string]disposableContext{},
		views:      map[string][]*viewEntry{},
		initiators: map[string]string{},
		factories:  map[string]driver.Factory{},
	}
}

func (cm *manager) GetService(typ reflect.Type) (interface{}, error) {
	return cm.sp.GetService(typ)
}

func (cm *manager) RegisterFactory(id string, factory driver.Factory) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Register View Factory [%s,%t]", id, factory)
	}
	cm.factoriesSync.Lock()
	defer cm.factoriesSync.Unlock()
	cm.factories[id] = factory
	return nil
}

func (cm *manager) NewView(id string, in []byte) (f view.View, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("new view triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed creating view [%s]", r)
		}
	}()

	cm.factoriesSync.RLock()
	factory, ok := cm.factories[id]
	cm.factoriesSync.RUnlock()
	if !ok {
		return nil, errors.Errorf("no factory found for id [%s]", id)
	}
	return factory.NewView(in)
}

func (cm *manager) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return cm.RegisterResponderWithIdentity(responder, nil, initiatedBy)
}

func (cm *manager) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error {
	switch t := initiatedBy.(type) {
	case view.View:
		cm.registerResponderWithIdentity(responder, id, cm.GetIdentifier(t))
	case string:
		cm.registerResponderWithIdentity(responder, id, t)
	default:
		return errors.Errorf("initiatedBy must be a view or a string")
	}
	return nil
}

func (cm *manager) GetResponder(initiatedBy interface{}) (view.View, error) {
	var initiatedByID string
	switch t := initiatedBy.(type) {
	case view.View:
		initiatedByID = cm.GetIdentifier(t)
	case string:
		initiatedByID = t
	default:
		return nil, errors.Errorf("initiatedBy must be a view or a string")
	}

	cm.viewsSync.Lock()
	defer cm.viewsSync.Unlock()

	responderID, ok := cm.initiators[initiatedByID]
	if !ok {
		return nil, errors.Errorf("responder not found for [%s]", initiatedByID)
	}

	entries, ok := cm.views[responderID]
	if !ok {
		return nil, errors.Errorf("responder not found for [%s], initiator [%s]", responderID, initiatedByID)
	}
	if len(entries) == 0 {
		return nil, errors.Errorf("responder not found for [%s], initiator [%s]", responderID, initiatedByID)
	}
	return entries[0].View, nil
}

func (cm *manager) registerResponderWithIdentity(responder view.View, id view.Identity, initiatedByID string) {
	cm.viewsSync.Lock()
	defer cm.viewsSync.Unlock()

	responderID := getIdentifier(responder)
	logger.Debugf("registering responder [%s] for initiator [%s] with identity [%s]", responderID, initiatedByID, id)

	cm.views[responderID] = append(cm.views[responderID], &viewEntry{View: responder, ID: id, Initiator: len(initiatedByID) == 0})
	if len(initiatedByID) != 0 {
		cm.initiators[initiatedByID] = responderID
	}
}

func (cm *manager) Initiate(id string) (interface{}, error) {
	// Lookup the initiator
	cm.viewsSync.RLock()
	responders := cm.views[id]
	var res *viewEntry
	for _, entry := range responders {
		if entry.Initiator {
			res = entry
			break
		}
	}
	cm.viewsSync.RUnlock()
	if res == nil {
		return nil, errors.Errorf("initiator not found for [%s]", id)
	}

	return cm.InitiateViewWithIdentity(res.View, cm.me())
}

func (cm *manager) InitiateView(view view.View) (interface{}, error) {
	return cm.InitiateViewWithIdentity(view, cm.me())
}

func (cm *manager) InitiateViewWithIdentity(view view.View, id view.Identity) (interface{}, error) {
	// Create the context
	cm.contextsSync.Lock()
	ctx := cm.ctx
	cm.contextsSync.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	viewContext, err := NewContextForInitiator(ctx, cm.sp, GetCommLayer(cm.sp), driver.GetEndpointService(cm.sp), id, view)
	if err != nil {
		return nil, err
	}
	childContext := &childContext{ParentContext: viewContext}
	cm.contextsSync.Lock()
	cm.contexts[childContext.ID()] = childContext
	cm.contextsSync.Unlock()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s]", id, getIdentifier(view), childContext.ID())
	}
	res, err := childContext.RunView(view)
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s] failed [%s]", id, getIdentifier(view), childContext.ID(), err)
		}
		return nil, err
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] InitiateView [view:%s], [ContextID:%s] terminated", id, getIdentifier(view), childContext.ID())
	}
	return res, nil
}

func (cm *manager) InitiateContext(view view.View) (view.Context, error) {
	return cm.InitiateContextWithIdentity(view, cm.me())
}

func (cm *manager) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	// Create the context
	cm.contextsSync.Lock()
	ctx := cm.ctx
	cm.contextsSync.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	viewContext, err := NewContextForInitiator(ctx, cm.sp, GetCommLayer(cm.sp), driver.GetEndpointService(cm.sp), id, view)
	if err != nil {
		return nil, err
	}
	childContext := &childContext{ParentContext: viewContext}
	cm.contextsSync.Lock()
	cm.contexts[childContext.ID()] = childContext
	cm.contextsSync.Unlock()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] InitiateContext [view:%s], [ContextID:%s]\n", id, getIdentifier(view), childContext.ID())
	}

	return childContext, nil
}

func (cm *manager) Start(ctx context.Context) {
	cm.contextsSync.Lock()
	cm.ctx = ctx
	cm.contextsSync.Unlock()
	session, err := GetCommLayer(cm.sp).MasterSession()
	if err != nil {
		return
	}
	for {
		ch := session.Receive()
		select {
		case msg := <-ch:
			go cm.callView(msg)
		case <-ctx.Done():
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("received done signal, stopping listening to messages on the master session")
			}
			return
		}
	}
}

func (cm *manager) Context(contextID string) (view.Context, error) {
	cm.contextsSync.RLock()
	defer cm.contextsSync.RUnlock()
	context, ok := cm.contexts[contextID]
	if !ok {
		return nil, errors.Errorf("context %s not found", contextID)
	}
	return context, nil
}

func (cm *manager) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	var ids []view.Identity
	resolver := driver.GetEndpointService(cm.sp)
	for _, endpoint := range endpoints {
		id, err := resolver.GetIdentity(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the idnetity at %s", endpoint)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func (cm *manager) GetIdentifier(f view.View) string {
	return getIdentifier(f)
}

func (cm *manager) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	cm.viewsSync.RLock()
	defer cm.viewsSync.RUnlock()

	// Is there a responder
	label, ok := cm.initiators[caller]
	if !ok {
		return nil, nil, errors.Errorf("no view found initiatable by [%s]", caller)
	}
	responders := cm.views[label]
	var res *viewEntry
	for _, entry := range responders {
		if !entry.Initiator {
			res = entry
		}
	}
	if res == nil {
		return nil, nil, errors.Errorf("responder not found for [%s]", label)
	}

	return res.View, res.ID, nil
}

func (cm *manager) respond(responder view.View, id view.Identity, msg *view.Message) (ctx view.Context, res interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("respond triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("failed responding [%s]", r)
		}
	}()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] Respond [from:%s], [sessionID:%s], [contextID:%s], [view:%s]", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, getIdentifier(responder))
	}

	// get context
	var isNew bool
	ctx, isNew, err = cm.newContext(id, msg)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed getting context for [%s,%s,%v]", msg.ContextID, id, msg)
	}

	// todo: if a new contxt has been created to run the responder,
	// then dispose the context when the responder terminates
	// run view
	if isNew {
		// delete context at the end of the execution
		res, err = func(ctx view.Context, responder view.View) (interface{}, error) {
			defer func() {
				cm.deleteContext(id, ctx.ID())
			}()
			return ctx.RunView(responder)
		}(ctx, responder)
	} else {
		res, err = ctx.RunView(responder)
	}
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] Respond Failure [from:%s], [sessionID:%s], [contextID:%s] [%s]\n", id, msg.FromEndpoint, msg.SessionID, msg.ContextID, err)
		}
	}
	return ctx, res, err
}

func (cm *manager) newContext(id view.Identity, msg *view.Message) (view.Context, bool, error) {
	cm.contextsSync.Lock()
	defer cm.contextsSync.Unlock()

	isNew := false
	caller, err := driver.GetEndpointService(cm.sp).GetIdentity(msg.FromEndpoint, msg.FromPKID)
	if err != nil {
		return nil, false, err
	}

	contextID := msg.ContextID
	viewContext, ok := cm.contexts[contextID]
	if ok && viewContext.Session() != nil && viewContext.Session().Info().ID != msg.SessionID {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf(
				"[%s] Found context with different session id, recreate [contextID:%s, sessionIds:%s,%s]\n",
				id,
				msg.ContextID,
				msg.SessionID,
				viewContext.Session().Info().ID,
			)
		}
		delete(cm.contexts, contextID)
		ok = false
	}
	if !ok {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] Create new context to respond [contextID:%s]\n", id, msg.ContextID)
		}
		backend, err := GetCommLayer(cm.sp).NewSessionWithID(msg.SessionID, contextID, msg.FromEndpoint, msg.FromPKID, caller, msg)
		if err != nil {
			return nil, false, err
		}
		ctx := cm.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		newCtx, err := NewContext(ctx, cm.sp, contextID, GetCommLayer(cm.sp), driver.GetEndpointService(cm.sp), id, backend, caller)
		if err != nil {
			return nil, false, err
		}
		childContext := &childContext{ParentContext: newCtx}
		cm.contexts[contextID] = childContext
		viewContext = childContext
		isNew = true
	} else {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("[%s] No new context to respond, reuse [contextID:%s]\n", id, msg.ContextID)
		}
	}

	return viewContext, isNew, nil
}

func (cm *manager) deleteContext(id view.Identity, contextID string) {
	cm.contextsSync.Lock()
	defer cm.contextsSync.Unlock()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] Delete context [contextID:%s]\n", id, contextID)
	}
	// dispose context
	if context, ok := cm.contexts[contextID]; ok {
		context.Dispose()
		delete(cm.contexts, contextID)
	}
}

func (cm *manager) existResponder(msg *view.Message) (view.View, view.Identity, error) {
	return cm.ExistResponderForCaller(msg.Caller)
}

func (cm *manager) callView(msg *view.Message) {
	responder, id, err := cm.existResponder(msg)
	if err != nil {
		// TODO: No responder exists for this message
		// Let's cache it for a while an re-post
		logger.Errorf("[%s] No responder exists for [%s]: [%s]", cm.me(), msg.String(), err)
		return
	}
	if id.IsNone() {
		id = cm.me()
	}

	ctx, _, err := cm.respond(responder, id, msg)
	if err != nil {
		logger.Errorf("failed responding [%v, %v], err: [%s]", getIdentifier(responder), msg.String(), err)
		if ctx == nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("no context set, returning")
			}
			return
		}

		// Return the error to the caller
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("return the error to the caller [%s]", err)
		}
		err = ctx.Session().SendError([]byte(err.Error()))
		if err != nil {
			logger.Errorf(err.Error())
		}
	}
}

func (cm *manager) me() view.Identity {
	return driver.GetIdentityProvider(cm.sp).DefaultIdentity()
}

func getIdentifier(f view.View) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "/" + t.Name()
}
