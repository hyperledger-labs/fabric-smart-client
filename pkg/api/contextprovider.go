/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ContextProvider interface {
	InitiateContext(view view.View) (view.Context, error)
	InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error)
	Context(contextID string) (view.Context, error)
}
