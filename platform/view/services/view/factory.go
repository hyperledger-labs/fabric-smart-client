/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

// Factory is used to create instances of the View interface
type Factory interface {
	// NewView returns an instance of the View interface build using the passed argument.
	NewView(in []byte) (view.View, error)
}

// LocalFactory is used to create instances of the View interface
type LocalFactory interface {
	// NewViewWithArg returns an instance of the View interface build using the passed argument.
	NewViewWithArg(arg any) (view.View, error)
}
