/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

//go:generate counterfeiter -o mock/factory.go -fake-name Factory . Factory

// Factory is used to create instances of the View interface.
type Factory interface {
	// NewView returns an instance of the View interface built using the passed argument.
	NewView(in []byte) (view.View, error)
}
