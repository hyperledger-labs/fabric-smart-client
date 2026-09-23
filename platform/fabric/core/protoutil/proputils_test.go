/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func TestComputeProposalTxID(t *testing.T) {
	t.Parallel()
	txid := protoutil.ComputeTxID([]byte{1}, []byte{1})

	// Compute the function computed by ComputeTxID,
	// namely, base64(sha256(nonce||creator))
	hf := sha256.New()
	hf.Write([]byte{1})
	hf.Write([]byte{1})
	hashOut := hf.Sum(nil)
	txid2 := hex.EncodeToString(hashOut)

	t.Logf("%x\n", hashOut)
	t.Logf("%s\n", txid)
	t.Logf("%s\n", txid2)

	require.Equal(t, txid, txid2)
}

// TestGetBytesRejectsNil checks each GetBytes helper reports a nil message rather than
// marshalling it.
func TestGetBytesRejectsNil(t *testing.T) {
	t.Parallel()

	_, err := protoutil.GetBytesChaincodeProposalPayload(nil)
	require.Error(t, err)
	_, err = protoutil.GetBytesChaincodeActionPayload(nil)
	require.Error(t, err)
	_, err = protoutil.GetBytesTransaction(nil)
	require.Error(t, err)
	_, err = protoutil.GetBytesPayload(nil)
	require.Error(t, err)
}

// TestGetBytesProposalResponsePayload checks the payload wraps a ChaincodeAction carrying
// every input.
func TestGetBytesProposalResponsePayload(t *testing.T) {
	t.Parallel()

	hash := []byte("hash")
	resp := &peer.Response{Status: 200, Message: "ok"}
	ccid := &peer.ChaincodeID{Name: "mycc"}

	raw, err := protoutil.GetBytesProposalResponsePayload(hash, resp, []byte("result"), []byte("event"), ccid)
	require.NoError(t, err)

	prp, err := protoutil.UnmarshalProposalResponsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, hash, prp.ProposalHash)

	act, err := protoutil.UnmarshalChaincodeAction(prp.Extension)
	require.NoError(t, err)
	assert.Equal(t, []byte("result"), act.Results)
	assert.Equal(t, []byte("event"), act.Events)
	assert.True(t, proto.Equal(resp, act.Response))
	assert.True(t, proto.Equal(ccid, act.ChaincodeId))
}

// TestCreateChaincodeProposalWithTxIDNonceAndTransient checks the given txid, channel,
// creator, nonce and transient map all land in the proposal.
func TestCreateChaincodeProposalWithTxIDNonceAndTransient(t *testing.T) {
	t.Parallel()

	cis := &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{ChaincodeId: &peer.ChaincodeID{Name: "mycc"}}}
	transient := map[string][]byte{"key": []byte("secret")}

	prop, txid, err := protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(
		"tx1", common.HeaderType_ENDORSER_TRANSACTION, "mychannel", cis, []byte("nonce"), []byte("creator"), transient)
	require.NoError(t, err)
	assert.Equal(t, "tx1", txid)

	hdr, err := protoutil.UnmarshalHeader(prop.Header)
	require.NoError(t, err)

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	require.NoError(t, err)
	assert.Equal(t, "mychannel", chdr.ChannelId)
	assert.Equal(t, "tx1", chdr.TxId)
	assert.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), chdr.Type)

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	require.NoError(t, err)
	assert.Equal(t, []byte("creator"), shdr.Creator)
	assert.Equal(t, []byte("nonce"), shdr.Nonce)

	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(prop.Payload)
	require.NoError(t, err)
	assert.Equal(t, transient, cpp.TransientMap)
}
