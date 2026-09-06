/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestIdentityProvider(t *testing.T) {
	t.Parallel()

	mockLocalMembership := &mock.LocalMembership{}
	mockIP := &mock.IdentityProvider{}
	ip := &IdentityProvider{localMembership: mockLocalMembership, ip: mockIP}

	mockLocalMembership.DefaultIdentityReturns([]byte("default-id"))
	require.Equal(t, view.Identity("default-id"), ip.DefaultIdentity())
	require.Equal(t, 1, mockLocalMembership.DefaultIdentityCallCount())

	mockIP.IdentityReturns([]byte("id-1"), nil)
	id, err := ip.Identity("label-1")
	require.NoError(t, err)
	require.Equal(t, view.Identity("id-1"), id)
	require.Equal(t, 1, mockIP.IdentityCallCount())
	require.Equal(t, "label-1", mockIP.IdentityArgsForCall(0))

	mockIP.IdentityReturns(nil, errors.New("identity err"))
	_, err = ip.Identity("label-2")
	require.ErrorContains(t, err, "identity err")
}
