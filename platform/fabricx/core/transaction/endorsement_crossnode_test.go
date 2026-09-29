/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"testing"

	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	qsmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

// newVaultWithNsVersion returns a real fabricx vault whose query service reports
// version for the _meta entry of ns1.
func newVaultWithNsVersion(version uint64) *vault.Vault {
	qs := &qsmock.QueryService{}
	qs.GetStatesReturns(map[cdriver.Namespace]map[cdriver.PKey]cdriver.VaultValue{
		"_meta": {"ns1": {Version: vault.MarshalVersion(version)}},
	}, nil)
	return vault.NewVault(qs, nil)
}

// TestGetProposalResponse_CrossNodePayloadEquality guards against issue #1377.
//
// The endorsement collection flow (collectEndorsementsView.Call in
// platform/fabric/services/endorser/endorsement.go) rejects an endorsement with
// "received different results" unless every endorser's proposal response payload is
// byte-equal to the issuer's. An approver rebuilds the RWSet from the bytes the issuer
// ships and re-serializes it in getProposalResponse. If serialization resolved
// namespace versions from the local node's view of _meta, two nodes that disagree on a
// namespace version would produce different payloads for the same transaction.
// Commit ed892d3a (#1647) pins namespace versions into the RWSet at first touch and
// carries them through NewRWSetFromBytes; this test checks that guarantee end to end
// through getProposalResponse using two real vaults that report different versions.
func TestGetProposalResponse_CrossNodePayloadEquality(t *testing.T) {
	t.Parallel()
	const txID = "tx-1377"
	ctx := t.Context()

	_, signerIdentityRaw := mustSerializedIdentityWithRealCert(t, "Org1MSP")
	signer := &testSerializableSigner{creator: signerIdentityRaw, signRes: []byte("signature-data")}
	sp, err := newSignedProposal(testSignedProposalBytes(t))
	require.NoError(t, err)

	// Issuer: sees ns1 at version 7.
	issuerRWS, err := newVaultWithNsVersion(7).NewRWSet(ctx, txID)
	require.NoError(t, err)
	require.NoError(t, issuerRWS.SetState("ns1", "key1", []byte("value1")))
	issuerResp, err := (&Transaction{TTxID: txID, signedProposal: sp, rwSetHandle: issuerRWS}).getProposalResponse(signer)
	require.NoError(t, err)

	rawRWSet, err := issuerRWS.Bytes()
	require.NoError(t, err)

	// Approver: sees ns1 at version 9 and rebuilds the RWSet from the issuer's bytes.
	approverRWS, err := newVaultWithNsVersion(9).NewRWSetFromBytes(ctx, txID, rawRWSet)
	require.NoError(t, err)
	approverResp, err := (&Transaction{TTxID: txID, signedProposal: sp, rwSetHandle: approverRWS}).getProposalResponse(signer)
	require.NoError(t, err)

	require.Equal(t, issuerResp.Payload, approverResp.Payload,
		"issue #1377: approver must return the issuer's exact results bytes, "+
			"otherwise collectEndorsementsView rejects the endorsement with 'received different results'")

	var tx applicationpb.Tx
	require.NoError(t, proto.Unmarshal(approverResp.Payload, &tx))
	require.Len(t, tx.GetNamespaces(), 1)
	require.Equal(t, uint64(7), tx.GetNamespaces()[0].GetNsVersion(), "approver must keep the issuer's pinned ns version")
}
