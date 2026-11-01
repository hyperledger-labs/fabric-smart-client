/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
)

// This file fuzzes the decoders that read bytes another organization controls on Fabric-x:
// the proposal responses an endorsement collector receives and the envelopes a responder
// receives. Each target either fails or returns a value whose accessors don't panic.

// FuzzFabricxNewProposalResponseFromBytes fuzzes (*Manager).NewProposalResponseFromBytes,
// which parses the proposal responses an endorsement collector receives.
func FuzzFabricxNewProposalResponseFromBytes(f *testing.F) {
	tx, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns"}}})
	require.NoError(f, err)
	signature, err := marshalEndorsementsForProposalResponse([]*applicationpb.Endorsements{sampleNamespaceEndorsements("Org1MSP", "sig")})
	require.NoError(f, err)
	valid, err := proto.Marshal(&peer.ProposalResponse{
		Payload:     tx,
		Endorsement: &peer.Endorsement{Endorser: []byte("endorser"), Signature: signature},
		Response:    &peer.Response{Status: 200, Payload: []byte("tx1")},
	})
	require.NoError(f, err)

	f.Add(valid)
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not a protobuf message"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		pr, err := (&Manager{}).NewProposalResponseFromBytes(raw)
		if err != nil {
			return
		}
		_ = pr.Endorser()
		_ = pr.EndorserSignature()
		_ = pr.Payload()
		_ = pr.Results()
		_ = pr.ResponseStatus()
		_ = pr.ResponseMessage()
		_, err = pr.Bytes()
		require.NoError(t, err)
		fxpr, ok := pr.(*ProposalResponse)
		require.True(t, ok)
		_ = fxpr.PR()
		provider := &mock.VerifierProvider{}
		provider.GetVerifierReturns(&mock.Verifier{}, nil)
		_ = fxpr.VerifyEndorsement(provider)
	})
}

// FuzzFabricxUnpackEnvelopeFromBytes fuzzes UnpackEnvelopeFromBytes, which decodes the
// envelope a responder receives from another node.
func FuzzFabricxUnpackEnvelopeFromBytes(f *testing.F) {
	chdr, err := proto.Marshal(&common.ChannelHeader{Type: int32(common.HeaderType_MESSAGE), TxId: "tx1"})
	require.NoError(f, err)
	payload, err := proto.Marshal(&common.Payload{Header: &common.Header{ChannelHeader: chdr}, Data: []byte("results")})
	require.NoError(f, err)
	valid, err := proto.Marshal(&common.Envelope{Payload: payload})
	require.NoError(f, err)
	headerless, err := proto.Marshal(&common.Envelope{Payload: []byte{}})
	require.NoError(f, err)

	f.Add(valid)
	f.Add(headerless)
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not a protobuf message"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		upe, typ, err := UnpackEnvelopeFromBytes(raw)
		if err != nil {
			return
		}
		require.Equal(t, int32(common.HeaderType_MESSAGE), typ)
		_ = upe.ID()
	})
}
