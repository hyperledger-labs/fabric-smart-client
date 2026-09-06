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
)

func TestACLProvider(t *testing.T) {
	t.Parallel()

	t.Run("CheckACL success", func(t *testing.T) {
		t.Parallel()

		mockChannelMembership := &mock.ChannelMembership{}
		mockChannelMembership.CheckACLReturns(nil)

		aclProvider := NewACLProvider(mockChannelMembership)

		mockSignedProp := &mock.SignedProposal{}
		signedProp := &SignedProposal{s: mockSignedProp}

		err := aclProvider.CheckACL(signedProp)
		require.NoError(t, err)

		require.Equal(t, 1, mockChannelMembership.CheckACLCallCount())
		require.Equal(t, mockSignedProp, mockChannelMembership.CheckACLArgsForCall(0))
	})

	t.Run("CheckACL error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("acl check failed")
		mockChannelMembership := &mock.ChannelMembership{}
		mockChannelMembership.CheckACLReturns(expectedErr)

		aclProvider := NewACLProvider(mockChannelMembership)

		mockSignedProp := &mock.SignedProposal{}
		signedProp := &SignedProposal{s: mockSignedProp}

		err := aclProvider.CheckACL(signedProp)
		require.ErrorContains(t, err, "acl check failed")
	})
}
