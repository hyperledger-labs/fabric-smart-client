/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

// View wraps a callable function.
type View interface {
	// Call invokes the View on input the passed argument.
	// It returns a result and error in case of failure.
	Call(context Context) (interface{}, error)
}

type TypedView[T any] interface {
	View
	CallTyped(context Context) (T, error)
}

func RunTypedView[T any, V TypedView[T]](context Context, v V, opts ...RunViewOption) (T, error) {
	return v.CallTyped(context)
}
