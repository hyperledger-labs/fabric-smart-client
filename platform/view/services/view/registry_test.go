/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Hello struct {
	Message string
}

type HelloView struct {
	Hello
}

func (h *HelloView) Call(context view2.Context) (any, error) {
	return h.Message, nil
}

type HelloLocalFactory struct{}

func (h *HelloLocalFactory) NewViewWithArg(arg any) (view2.View, error) {
	hello, ok := arg.(*Hello)
	if !ok || arg == nil {
		return nil, errors.New("arg is not a Hello")
	}
	return &HelloView{Hello: *hello}, nil
}

func TestNewLocalView(t *testing.T) {
	registry := view.NewRegistry()
	require.NoError(t, registry.RegisterLocalFactory("hello", &HelloLocalFactory{}))

	view, err := registry.NewLocalView("hello", &Hello{Message: "hello world"})
	require.NoError(t, err)
	msgBoxed, err := view.Call(nil)
	require.NoError(t, err)
	msg, ok := msgBoxed.(string)
	assert.True(t, ok)
	assert.Equal(t, msg, "hello world")
}
