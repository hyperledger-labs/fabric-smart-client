/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
)

func TestSignedProposalAccessors(t *testing.T) {
	t.Parallel()

	raw := testSignedProposalBytes(t)
	sp, err := newSignedProposal(raw)
	require.NoError(t, err)

	require.Equal(t, raw.ProposalBytes, sp.ProposalBytes())
	require.Equal(t, []byte("sig"), sp.Signature())
	require.NotEmpty(t, sp.ProposalHash())
	require.Equal(t, "cc", sp.ChaincodeName())
	require.Equal(t, "v1", sp.ChaincodeVersion())
	require.Same(t, raw, sp.Internal())
}

func TestNewSignedProposalInvalid(t *testing.T) {
	t.Parallel()

	sp, err := newSignedProposal(&peer.SignedProposal{ProposalBytes: []byte("garbage")})
	require.Error(t, err)
	require.Nil(t, sp)
}
