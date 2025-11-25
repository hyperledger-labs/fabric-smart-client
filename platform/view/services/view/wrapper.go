/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
)

// WrappedContext wraps an existing view context to provider a different context.Context
type WrappedContext struct {
	ParentContext
	ctx context.Context
}

// WrapContext returns a new WrappedContext for the given arguments
func WrapContext(parent ParentContext, ctx context.Context) *WrappedContext {
	return &WrappedContext{
		ParentContext: parent,
		ctx:           ctx,
	}
}

// Context returns the overrode go context
func (c *WrappedContext) Context() context.Context {
	return c.ctx
}
