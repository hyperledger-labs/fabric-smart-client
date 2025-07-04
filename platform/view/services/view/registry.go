/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type viewEntry struct {
	View      view.View
	ID        view.Identity
	Initiator bool
}

type Registry struct {
	factoriesSync sync.RWMutex
	viewsSync     sync.RWMutex

	views      map[string][]*viewEntry
	initiators map[string]string
	factories  map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{
		views:      map[string][]*viewEntry{},
		initiators: map[string]string{},
		factories:  map[string]Factory{},
	}
}

func (cm *Registry) RegisterFactory(id string, factory Factory) error {
	logger.Debugf("Register View Factory [%s,%t]", id, factory)
	cm.factoriesSync.Lock()
	defer cm.factoriesSync.Unlock()
	cm.factories[id] = factory
	return nil
}

func (cm *Registry) NewView(id string, in []byte) (f view.View, err error) {
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

func (cm *Registry) RegisterResponderFactory(factory Factory, initiatedBy interface{}) error {
	responder, err := factory.NewView(nil)
	if err != nil {
		return err
	}
	return cm.RegisterResponder(responder, initiatedBy)
}

func (cm *Registry) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return cm.RegisterResponderWithIdentity(responder, nil, initiatedBy)
}

func (cm *Registry) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error {
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

func (cm *Registry) GetResponder(initiatedBy interface{}) (view.View, error) {
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
	// Recall that a responder can be used to respond to multiple initiators.
	// Therefore, all these entries are for the same responder.
	// We return the first one.
	return entries[0].View, nil
}

func (cm *Registry) GetIdentifier(f view.View) string {
	return GetIdentifier(f)
}

func (cm *Registry) registerResponderWithIdentity(responder view.View, id view.Identity, initiatedByID string) {
	cm.viewsSync.Lock()
	defer cm.viewsSync.Unlock()

	responderID := GetIdentifier(responder)
	logger.Debugf("registering responder [%s] for initiator [%s] with identity [%s]", responderID, initiatedByID, id)

	cm.views[responderID] = append(cm.views[responderID], &viewEntry{View: responder, ID: id, Initiator: len(initiatedByID) == 0})
	if len(initiatedByID) != 0 {
		cm.initiators[initiatedByID] = responderID
	}
}

func (cm *Registry) GetView(id string) (view.View, error) {
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
	return res.View, nil
}

func (cm *Registry) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
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

func GetIdentifier(f view.View) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "/" + t.Name()
}

func GetName(f view.View) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// GetRegistry returns an instance of the view registry.
// It panics, if no instance is found.
func GetRegistry(sp services.Provider) *Registry {
	s, err := sp.GetService(reflect.TypeOf((*Registry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(*Registry)
}
