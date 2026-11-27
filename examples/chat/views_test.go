/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat

import (
	"testing"

	"github.com/stretchr/testify/require"

	viewservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestOnlyFrom(t *testing.T) {
	t.Parallel()
	auth := OnlyFrom(view.Identity("server"))

	require.NoError(t, auth.Authorize(t.Context(), view.Identity("server")))
	require.ErrorIs(t, auth.Authorize(t.Context(), view.Identity("bob")), viewservice.ErrDenied)
	require.ErrorIs(t, auth.Authorize(t.Context(), nil), viewservice.ErrDenied)
}

// TestDeliverOnlyFromServer checks that the client registers its channel-message responder
// with an authorizer that admits the server and nobody else.
func TestDeliverOnlyFromServer(t *testing.T) {
	t.Parallel()
	registry := viewservice.NewRegistry()
	require.NoError(t, registerResponders(registry, NewInbox(), view.Identity("server")))

	responder, _, auth, err := registry.ResponderForCaller("", viewservice.GetIdentifier(&Push{}))
	require.NoError(t, err)
	require.IsType(t, &Deliver{}, responder)
	require.NotNil(t, auth)
	require.NoError(t, auth.Authorize(t.Context(), view.Identity("server")))
	require.ErrorIs(t, auth.Authorize(t.Context(), view.Identity("bob")), viewservice.ErrDenied)

	_, _, auth, err = registry.ResponderForCaller("", viewservice.GetIdentifier(&DM{}))
	require.NoError(t, err)
	require.Nil(t, auth, "any client may send a direct message")
}
