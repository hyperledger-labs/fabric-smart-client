/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Context struct {
	c view.Context
}

func (c *Context) RunView(f View) (interface{}, error) {
	return c.c.RunView(f)
}

func (c *Context) ID() string {
	return c.c.ID()
}

type Manager struct {
	m api.ViewManager
}

func (m *Manager) NewView(id string, in []byte) (View, error) {
	return m.m.NewView(id, in)
}

func (m *Manager) Context(contextID string) (*Context, error) {
	context, err := m.m.Context(contextID)
	if err != nil {
		return nil, err
	}
	return &Context{c: context}, nil
}

func (m *Manager) InitiateView(view View) (interface{}, error) {
	return m.m.InitiateView(view)
}

func (m *Manager) InitiateContext(view View) (*Context, error) {
	context, err := m.m.InitiateContext(view)
	if err != nil {
		return nil, err
	}
	return &Context{c: context}, nil
}

func GetManager(sp ServiceProvider) *Manager {
	return &Manager{m: api.GetViewManager(sp)}
}
