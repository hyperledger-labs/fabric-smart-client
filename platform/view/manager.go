/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Context gives a view information about the environment in which it is in execution
type Context struct {
	view.Context
}

// Manager manages the lifecycle of views and contexts
type Manager struct {
	m driver.ViewManager
}

// NewView returns a new instance of the view identified by the passed id and on input.
// Have a look at Registry to learn how to register view factories and responders
func (m *Manager) NewView(id string, in []byte) (View, error) {
	return m.m.NewView(id, in)
}

// Context returns the context associated to the passed id, an error if not context is found.
func (m *Manager) Context(contextID string) (*Context, error) {
	context, err := m.m.Context(contextID)
	if err != nil {
		return nil, err
	}
	return &Context{Context: context}, nil
}

// InitiateView invokes the passed view and returns the result produced by that view
func (m *Manager) InitiateView(view View) (interface{}, error) {
	return m.m.InitiateView(view)
}

// InitiateContext initiates a new context for the passed view
func (m *Manager) InitiateContext(view View) (*Context, error) {
	context, err := m.m.InitiateContext(view)
	if err != nil {
		return nil, err
	}
	return &Context{Context: context}, nil
}

// InitiateContextWithIdentityAndID initiates
func (m *Manager) InitiateContextWithIdentityAndID(view View, id view.Identity, contextID string) (view.Context, error) {
	context, err := m.m.InitiateContextWithIdentityAndID(view, id, contextID)
	if err != nil {
		return nil, err
	}
	return context, nil
}

// GetManager returns an instance of the view manager.
// It panics, if no instance is found.
func GetManager(sp ServiceProvider) *Manager {
	return NewManager(driver.GetViewManager(sp))
}

func NewManager(m driver.ViewManager) *Manager {
	return &Manager{m: m}
}
