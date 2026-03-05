/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
)

func TestChildContext(t *testing.T) {
	parent := &mock.MutableParentContext{}
	parent.IDReturns("parent-id")
	parent.MeReturns(view2.Identity("me"))
	
	child := view.NewChildContextFromParent(parent)
	assert.Equal(t, "parent-id", child.ID())
	assert.Equal(t, view2.Identity("me"), child.Me())
	
	session := &mock.Session{}
	child2 := view.NewChildContextFromParentAndSession(parent, session)
	assert.Equal(t, session, child2.Session())
	
	initiator := &mock.View{}
	child3 := view.NewChildContextFromParentAndInitiator(parent, initiator)
	assert.Equal(t, initiator, child3.Initiator())
	
	child4 := view.NewChildContext(parent, session, initiator)
	assert.Equal(t, session, child4.Session())
	assert.Equal(t, initiator, child4.Initiator())
	
	// Test methods that delegate to parent
	child4.StartSpanFrom(context.Background(), "test")
	assert.Equal(t, 1, parent.StartSpanFromCallCount())
	
	child4.GetService(reflect.TypeOf(""))
	assert.Equal(t, 1, parent.GetServiceCallCount())
	
	child4.PutService("test")
	assert.Equal(t, 1, parent.PutServiceCallCount())
	
	child4.IsMe(view2.Identity("me"))
	assert.Equal(t, 1, parent.IsMeCallCount())
	
	child4.GetSession(nil, view2.Identity("party"))
	assert.Equal(t, 1, parent.GetSessionCallCount())
	
	child4.GetSessionByID("sid", view2.Identity("party"))
	assert.Equal(t, 1, parent.GetSessionByIDCallCount())
	
	child4.Context()
	assert.Equal(t, 1, parent.ContextCallCount())
	
	child4.ResetSessions()
	assert.Equal(t, 1, parent.ResetSessionsCallCount())
	
	called := false
	child4.OnError(func() { called = true })
	child4.Cleanup()
	assert.True(t, called)
	
	child4.Dispose()
	assert.Equal(t, 1, parent.DisposeCallCount())
	
	child4.PutSession(nil, nil, nil)
	assert.Equal(t, 1, parent.PutSessionCallCount())
	
	child4.PutSessionByID("", nil, nil)
	assert.Equal(t, 1, parent.PutSessionByIDCallCount())
}
