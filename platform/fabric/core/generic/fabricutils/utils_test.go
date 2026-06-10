/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricutils

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func mustMarshalProto(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(msg)
	require.NoError(t, err)
	return raw
}

func buildRWSetBytes(t *testing.T, key string, value []byte) []byte {
	t.Helper()
	txRWSet := &rwset.TxReadWriteSet{
		NsRwset: []*rwset.NsReadWriteSet{
			{
				Namespace: "ns1",
				Rwset: mustMarshalProto(t, &kvrwset.KVRWSet{
					Writes: []*kvrwset.KVWrite{
						{Key: key, Value: value},
					},
				}),
			},
		},
	}
	return mustMarshalProto(t, txRWSet)
}

func buildProposalResponsePayload(t *testing.T, rwsetBytes []byte) []byte {
	t.Helper()
	payload, err := protoutil.GetBytesProposalResponsePayload(
		[]byte("proposal-hash"),
		&peer.Response{Status: 200, Message: "OK"},
		rwsetBytes,
		nil,
		&peer.ChaincodeID{Name: "mycc", Version: "v1"},
	)
	require.NoError(t, err)
	return payload
}

func buildProposal(t *testing.T, creator []byte, channel string) *fake.Proposal {
	t.Helper()
	nonce := []byte("nonce")
	txID := protoutil.ComputeTxID(nonce, creator)
	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			Type:        peer.ChaincodeSpec_GOLANG,
			ChaincodeId: &peer.ChaincodeID{Name: "mycc", Version: "v1"},
			Input: &peer.ChaincodeInput{
				Args: [][]byte{[]byte("invoke"), []byte("a"), []byte("b")},
			},
		},
	}

	prop, _, err := protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(
		txID,
		common.HeaderType_ENDORSER_TRANSACTION,
		channel,
		cis,
		nonce,
		creator,
		nil,
	)
	require.NoError(t, err)

	return &fake.Proposal{
		HeaderBytes:  prop.Header,
		PayloadBytes: prop.Payload,
	}
}

func buildResponse(status int32, message string, payload []byte) driver.ProposalResponse {
	return &fake.ProposalResponse{
		EndorserBytes:          []byte("endorser"),
		PayloadBytes:           payload,
		EndorserSignatureBytes: []byte("endorser-signature"),
		Status:                 status,
		Message:                message,
	}
}

func buildEndorserEnvelopeBytes(t *testing.T) []byte {
	t.Helper()
	creator := []byte("creator")
	signer := &fake.SerializableSigner{Serialized: creator, Signature: []byte("signature")}
	proposal := buildProposal(t, creator, "mychannel")
	resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))

	env, err := CreateEndorserSignedTX(signer, proposal, resp)
	require.NoError(t, err)
	return mustMarshalProto(t, env)
}

func TestUnmarshalTx(t *testing.T) {
	t.Parallel()

	t.Run("invalid envelope bytes", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := UnmarshalTx([]byte("bad-envelope"))
		require.ErrorContains(t, err, "Error getting tx from block")
	})

	t.Run("invalid payload bytes", func(t *testing.T) {
		t.Parallel()
		env := &common.Envelope{Payload: []byte("bad-payload")}
		_, _, _, err := UnmarshalTx(mustMarshalProto(t, env))
		require.ErrorContains(t, err, "unmarshal payload failed")
	})

	t.Run("invalid channel header", func(t *testing.T) {
		t.Parallel()
		payl := &common.Payload{
			Header: &common.Header{ChannelHeader: []byte("bad-channel-header")},
			Data:   []byte("data"),
		}
		env := &common.Envelope{Payload: mustMarshalProto(t, payl)}
		_, _, _, err := UnmarshalTx(mustMarshalProto(t, env))
		require.ErrorContains(t, err, "unmarshal channel header failed")
	})

	t.Run("nil payload header", func(t *testing.T) {
		t.Parallel()
		payl := &common.Payload{Data: []byte("data")}
		env := &common.Envelope{Payload: mustMarshalProto(t, payl)}
		_, _, _, err := UnmarshalTx(mustMarshalProto(t, env))
		require.ErrorContains(t, err, "envelope must have a Header")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		env, payl, chdr, err := UnmarshalTx(buildEndorserEnvelopeBytes(t))
		require.NoError(t, err)
		require.NotNil(t, env)
		require.NotNil(t, payl)
		require.NotNil(t, chdr)
		require.Equal(t, "mychannel", chdr.ChannelId)
	})
}

func TestCreateEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("signer error", func(t *testing.T) {
		t.Parallel()
		signErr := errors.New("sign-failed")
		s := &fake.SerializableSigner{SignErr: signErr}
		_, err := CreateEnvelope(s, &common.Header{}, []byte("payload"))
		require.ErrorIs(t, err, signErr)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		s := &fake.SerializableSigner{Signature: []byte("sig")}
		env, err := CreateEnvelope(s, &common.Header{}, []byte("payload"))
		require.NoError(t, err)
		require.Equal(t, []byte("sig"), env.Signature)

		payl := &common.Payload{}
		require.NoError(t, proto.Unmarshal(env.Payload, payl))
		require.Equal(t, []byte("payload"), payl.Data)
	})
}

func TestCreateEndorserTX(t *testing.T) {
	t.Parallel()

	serializeErr := errors.New("serialize-failed")
	successCreator := []byte("creator")
	successResponsePayload := buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1")))
	tests := []struct {
		name   string
		setup  func(*testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse)
		assert func(*testing.T, *common.Header, []byte, error)
	}{
		{
			name: "no proposal responses",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := buildProposal(t, []byte("creator"), "mychannel")
				signer := &fake.SerializableSigner{Serialized: []byte("creator")}
				return signer, proposal, nil
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "at least one proposal response is required")
			},
		},
		{
			name: "invalid proposal header",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := &fake.Proposal{HeaderBytes: []byte("bad-header"), PayloadBytes: []byte("bad-payload")}
				signer := &fake.SerializableSigner{Serialized: []byte("creator")}
				resp := buildResponse(200, "OK", []byte("payload"))
				return signer, proposal, []driver.ProposalResponse{resp}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "error unmarshalling Header")
			},
		},
		{
			name: "invalid proposal payload",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				validProposal := buildProposal(t, []byte("creator"), "mychannel")
				proposal := &fake.Proposal{HeaderBytes: validProposal.Header(), PayloadBytes: []byte("bad-payload")}
				signer := &fake.SerializableSigner{Serialized: []byte("creator")}
				resp := buildResponse(200, "OK", []byte("payload"))
				return signer, proposal, []driver.ProposalResponse{resp}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "error unmarshalling ChaincodeProposalPayload")
			},
		},
		{
			name: "serialize signer error",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := buildProposal(t, []byte("creator"), "mychannel")
				signer := &fake.SerializableSigner{SerializeErr: serializeErr}
				resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
				return signer, proposal, []driver.ProposalResponse{resp}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorIs(t, err, serializeErr)
			},
		},
		{
			name: "signature header unmarshal error",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				validProposal := buildProposal(t, []byte("creator"), "mychannel")
				hdr := &common.Header{
					ChannelHeader:   []byte("channel-header"),
					SignatureHeader: []byte("bad-signature-header"),
				}
				proposal := &fake.Proposal{
					HeaderBytes:  mustMarshalProto(t, hdr),
					PayloadBytes: validProposal.Payload(),
				}
				signer := &fake.SerializableSigner{Serialized: []byte("creator")}
				resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
				return signer, proposal, []driver.ProposalResponse{resp}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "error unmarshalling SignatureHeader")
			},
		},
		{
			name: "signer mismatch",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := buildProposal(t, []byte("creator1"), "mychannel")
				signer := &fake.SerializableSigner{Serialized: []byte("creator2")}
				resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
				return signer, proposal, []driver.ProposalResponse{resp}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "signer must be the same as the one referenced in the header")
			},
		},
		{
			name: "response not successful",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := buildProposal(t, []byte("creator"), "mychannel")
				signer := &fake.SerializableSigner{Serialized: []byte("creator")}
				resp := buildResponse(500, "boom", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
				return signer, proposal, []driver.ProposalResponse{resp}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "proposal response was not successful")
			},
		},
		{
			name: "response payload mismatch",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := buildProposal(t, []byte("creator"), "mychannel")
				signer := &fake.SerializableSigner{Serialized: []byte("creator")}
				resp1 := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
				resp2 := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v2"))))
				return signer, proposal, []driver.ProposalResponse{resp1, resp2}
			},
			assert: func(t *testing.T, _ *common.Header, _ []byte, err error) {
				t.Helper()
				require.ErrorContains(t, err, "ProposalResponsePayloads do not match")
			},
		},
		{
			name: "success",
			setup: func(t *testing.T) (SerializableSigner, driver.Proposal, []driver.ProposalResponse) {
				t.Helper()
				proposal := buildProposal(t, successCreator, "mychannel")
				signer := &fake.SerializableSigner{Serialized: successCreator}
				resp1 := buildResponse(200, "OK", successResponsePayload)
				resp2 := buildResponse(200, "OK", successResponsePayload)
				return signer, proposal, []driver.ProposalResponse{resp1, resp2}
			},
			assert: func(t *testing.T, hdr *common.Header, txBytes []byte, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, hdr)
				require.NotEmpty(t, txBytes)

				chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
				require.NoError(t, err)
				require.Equal(t, "mychannel", chdr.ChannelId)

				shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
				require.NoError(t, err)
				require.Equal(t, successCreator, shdr.Creator)

				tx, err := protoutil.UnmarshalTransaction(txBytes)
				require.NoError(t, err)
				require.Len(t, tx.Actions, 1)
				require.Equal(t, hdr.SignatureHeader, tx.Actions[0].Header)

				actionPayload, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[0].Payload)
				require.NoError(t, err)
				require.Equal(t, successResponsePayload, actionPayload.Action.ProposalResponsePayload)
				require.Len(t, actionPayload.Action.Endorsements, 2)
				require.Equal(t, []byte("endorser"), actionPayload.Action.Endorsements[0].Endorser)
				require.Equal(t, []byte("endorser-signature"), actionPayload.Action.Endorsements[0].Signature)

				proposal := buildProposal(t, successCreator, "mychannel")
				proposalPayload, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.Payload())
				require.NoError(t, err)
				expectedPayload, err := protoutil.GetBytesProposalPayloadForTx(proposalPayload)
				require.NoError(t, err)
				require.Equal(t, expectedPayload, actionPayload.ChaincodeProposalPayload)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			signer, proposal, resps := tt.setup(t)
			hdr, txBytes, err := CreateEndorserTX(signer, proposal, resps...)
			tt.assert(t, hdr, txBytes, err)
		})
	}
}

func TestCreateEndorserSignedTX(t *testing.T) {
	t.Parallel()

	t.Run("propagates create endorser tx errors", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &fake.SerializableSigner{Serialized: []byte("creator")}
		_, err := CreateEndorserSignedTX(signer, proposal)
		require.ErrorContains(t, err, "at least one proposal response is required")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &fake.SerializableSigner{Serialized: []byte("creator"), Signature: []byte("sig")}
		resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))

		env, err := CreateEndorserSignedTX(signer, proposal, resp)
		require.NoError(t, err)
		require.NotNil(t, env)
		require.Equal(t, []byte("sig"), env.Signature)
		require.NotEmpty(t, env.Payload)
	})
}
