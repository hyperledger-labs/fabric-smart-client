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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type testSerializableSigner struct {
	serialized   []byte
	serializeErr error
	signature    []byte
	signErr      error
}

func (s *testSerializableSigner) Sign(_ []byte) ([]byte, error) {
	if s.signErr != nil {
		return nil, s.signErr
	}
	if len(s.signature) == 0 {
		return []byte("signature"), nil
	}
	return s.signature, nil
}

func (s *testSerializableSigner) Serialize() ([]byte, error) {
	if s.serializeErr != nil {
		return nil, s.serializeErr
	}
	return s.serialized, nil
}

type testSigner struct {
	signature []byte
	signErr   error
}

func (s *testSigner) Sign(_ []byte) ([]byte, error) {
	if s.signErr != nil {
		return nil, s.signErr
	}
	if len(s.signature) == 0 {
		return []byte("signature"), nil
	}
	return s.signature, nil
}

type testProposal struct {
	header  []byte
	payload []byte
}

func (p *testProposal) Header() []byte  { return p.header }
func (p *testProposal) Payload() []byte { return p.payload }

type testProposalResponse struct {
	endorser          []byte
	payload           []byte
	endorserSignature []byte
	results           []byte
	status            int32
	message           string
}

func (r *testProposalResponse) Endorser() []byte          { return r.endorser }
func (r *testProposalResponse) Payload() []byte           { return r.payload }
func (r *testProposalResponse) EndorserSignature() []byte { return r.endorserSignature }
func (r *testProposalResponse) Results() []byte           { return r.results }
func (r *testProposalResponse) ResponseStatus() int32     { return r.status }
func (r *testProposalResponse) ResponseMessage() string   { return r.message }
func (r *testProposalResponse) Bytes() ([]byte, error)    { return nil, nil }
func (r *testProposalResponse) VerifyEndorsement(driver.VerifierProvider) error {
	return nil
}

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

func buildProposal(t *testing.T, creator []byte, channel string) *testProposal {
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

	return &testProposal{
		header:  prop.Header,
		payload: prop.Payload,
	}
}

func buildResponse(status int32, message string, payload []byte) driver.ProposalResponse {
	return &testProposalResponse{
		endorser:          []byte("endorser"),
		payload:           payload,
		endorserSignature: []byte("endorser-signature"),
		status:            status,
		message:           message,
	}
}

func buildEndorserEnvelopeBytes(t *testing.T) []byte {
	t.Helper()
	creator := []byte("creator")
	signer := &testSerializableSigner{serialized: creator, signature: []byte("signature")}
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
		s := &testSigner{signErr: signErr}
		_, err := CreateEnvelope(s, &common.Header{}, []byte("payload"))
		require.ErrorIs(t, err, signErr)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		s := &testSigner{signature: []byte("sig")}
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

	t.Run("no proposal responses", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator")}
		_, _, err := CreateEndorserTX(signer, proposal)
		require.ErrorContains(t, err, "at least one proposal response is required")
	})

	t.Run("invalid proposal header", func(t *testing.T) {
		t.Parallel()
		proposal := &testProposal{header: []byte("bad-header"), payload: []byte("bad-payload")}
		signer := &testSerializableSigner{serialized: []byte("creator")}
		resp := buildResponse(200, "OK", []byte("payload"))
		_, _, err := CreateEndorserTX(signer, proposal, resp)
		require.Error(t, err)
	})

	t.Run("invalid proposal payload", func(t *testing.T) {
		t.Parallel()
		validProposal := buildProposal(t, []byte("creator"), "mychannel")
		proposal := &testProposal{header: validProposal.Header(), payload: []byte("bad-payload")}
		signer := &testSerializableSigner{serialized: []byte("creator")}
		resp := buildResponse(200, "OK", []byte("payload"))
		_, _, err := CreateEndorserTX(signer, proposal, resp)
		require.Error(t, err)
	})

	t.Run("serialize signer error", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		serializeErr := errors.New("serialize-failed")
		signer := &testSerializableSigner{serializeErr: serializeErr}
		resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
		_, _, err := CreateEndorserTX(signer, proposal, resp)
		require.ErrorIs(t, err, serializeErr)
	})

	t.Run("signature header unmarshal error", func(t *testing.T) {
		t.Parallel()
		validProposal := buildProposal(t, []byte("creator"), "mychannel")
		hdr := &common.Header{
			ChannelHeader:   []byte("channel-header"),
			SignatureHeader: []byte("bad-signature-header"),
		}
		proposal := &testProposal{
			header:  mustMarshalProto(t, hdr),
			payload: validProposal.Payload(),
		}
		signer := &testSerializableSigner{serialized: []byte("creator")}
		resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
		_, _, err := CreateEndorserTX(signer, proposal, resp)
		require.Error(t, err)
	})

	t.Run("signer mismatch", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator1"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator2")}
		resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
		_, _, err := CreateEndorserTX(signer, proposal, resp)
		require.ErrorContains(t, err, "signer must be the same as the one referenced in the header")
	})

	t.Run("response not successful", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator")}
		resp := buildResponse(500, "boom", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
		_, _, err := CreateEndorserTX(signer, proposal, resp)
		require.ErrorContains(t, err, "proposal response was not successful")
	})

	t.Run("response payload mismatch", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator")}

		resp1 := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))
		resp2 := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v2"))))
		_, _, err := CreateEndorserTX(signer, proposal, resp1, resp2)
		require.ErrorContains(t, err, "ProposalResponsePayloads do not match")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator")}

		payload := buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1")))
		resp1 := buildResponse(200, "OK", payload)
		resp2 := buildResponse(200, "OK", payload)

		hdr, txBytes, err := CreateEndorserTX(signer, proposal, resp1, resp2)
		require.NoError(t, err)
		require.NotNil(t, hdr)
		require.NotEmpty(t, txBytes)
	})
}

func TestCreateEndorserSignedTX(t *testing.T) {
	t.Parallel()

	t.Run("propagates create endorser tx errors", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator")}
		_, err := CreateEndorserSignedTX(signer, proposal)
		require.ErrorContains(t, err, "at least one proposal response is required")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		proposal := buildProposal(t, []byte("creator"), "mychannel")
		signer := &testSerializableSigner{serialized: []byte("creator"), signature: []byte("sig")}
		resp := buildResponse(200, "OK", buildProposalResponsePayload(t, buildRWSetBytes(t, "k1", []byte("v1"))))

		env, err := CreateEndorserSignedTX(signer, proposal, resp)
		require.NoError(t, err)
		require.NotNil(t, env)
		require.Equal(t, []byte("sig"), env.Signature)
		require.NotEmpty(t, env.Payload)
	})
}
