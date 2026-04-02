/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// WrappedContext wraps an existing view context to provide a different context.Context.
type WrappedContext struct {
	ParentContext
	ctx context.Context
}

// WrapContext returns a new WrappedContext for the given arguments.
func WrapContext(context ViewContext, ctx context.Context) *WrappedContext {
	parent, ok := context.(ParentContext)
	if !ok {
		panic("parent context is not a ParentContext")
	}
	return &WrappedContext{
		ParentContext: parent,
		ctx:           ctx,
	}
}

// Context returns the overridden go context.
func (c *WrappedContext) Context() context.Context {
	return c.ctx
}

// RunView runs the passed view on input this context.
func (c *WrappedContext) RunView(v view.View, opts ...view.RunViewOption) (res any, err error) {
	return RunViewNow(c, v, opts...)
}
