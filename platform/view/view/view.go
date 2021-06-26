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
