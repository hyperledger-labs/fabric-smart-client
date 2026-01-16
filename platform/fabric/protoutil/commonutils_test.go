/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil

import (
	"bytes"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
)

func TestNonceRandomness(t *testing.T) {
	n1, err := CreateNonce()
	if err != nil {
		t.Fatal(err)
	}
	n2, err := CreateNonce()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(n1, n2) {
		t.Fatalf("Expected nonces to be different, got %x and %x", n1, n2)
	}
}

func TestNonceLength(t *testing.T) {
	n, err := CreateNonce()
	if err != nil {
		t.Fatal(err)
	}
	actual := len(n)
	expected := NonceSize
	if actual != expected {
		t.Fatalf("Expected nonce to be of size %d, got %d instead", expected, actual)
	}
}

func TestUnmarshalPayload(t *testing.T) {
	var payload *cb.Payload
	good, _ := proto.Marshal(&cb.Payload{
		Data: []byte("payload"),
	})
	payload, err := UnmarshalPayload(good)
	require.NoError(t, err, "Unexpected error unmarshalling payload")
	require.NotNil(t, payload, "Payload should not be nil")
}

func TestUnmarshalSignatureHeader(t *testing.T) {
	t.Run("invalid header", func(t *testing.T) {
		sighdrBytes := []byte("invalid signature header")
		_, err := UnmarshalSignatureHeader(sighdrBytes)
		require.Error(t, err, "Expected unmarshalling error")
	})

	t.Run("valid empty header", func(t *testing.T) {
		sighdr := &cb.SignatureHeader{}
		sighdrBytes := MarshalOrPanic(sighdr)
		sighdr, err := UnmarshalSignatureHeader(sighdrBytes)
		require.NoError(t, err, "Unexpected error unmarshalling signature header")
		require.Nil(t, sighdr.Creator)
		require.Nil(t, sighdr.Nonce)
	})

	t.Run("valid header", func(t *testing.T) {
		sighdr := &cb.SignatureHeader{
			Creator: []byte("creator"),
			Nonce:   []byte("nonce"),
		}
		sighdrBytes := MarshalOrPanic(sighdr)
		sighdr, err := UnmarshalSignatureHeader(sighdrBytes)
		require.NoError(t, err, "Unexpected error unmarshalling signature header")
		require.Equal(t, []byte("creator"), sighdr.Creator)
		require.Equal(t, []byte("nonce"), sighdr.Nonce)
	})
}

func TestUnmarshalEnvelope(t *testing.T) {
	var env *cb.Envelope
	good, _ := proto.Marshal(&cb.Envelope{})
	env, err := UnmarshalEnvelope(good)
	require.NoError(t, err, "Unexpected error unmarshalling envelope")
	require.NotNil(t, env, "Envelope should not be nil")
}

func TestUnmarshalBlock(t *testing.T) {
	var env *cb.Block
	good, _ := proto.Marshal(&cb.Block{})
	env, err := UnmarshalBlock(good)
	require.NoError(t, err, "Unexpected error unmarshalling block")
	require.NotNil(t, env, "Block should not be nil")
}

func TestUnmarshalEnvelopeOfType(t *testing.T) {
	env := &cb.Envelope{}

	env.Payload = []byte("bad payload")
	_, err := UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	require.Error(t, err, "Expected error unmarshalling malformed envelope")

	payload, _ := proto.Marshal(&cb.Payload{
		Header: nil,
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	require.Error(t, err, "Expected error with missing payload header")

	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: []byte("bad header"),
		},
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	require.Error(t, err, "Expected error for malformed channel header")

	chdr, _ := proto.Marshal(&cb.ChannelHeader{
		Type: int32(cb.HeaderType_CHAINCODE_PACKAGE),
	})
	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdr,
		},
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, nil)
	require.Error(t, err, "Expected error for wrong channel header type")

	chdr, _ = proto.Marshal(&cb.ChannelHeader{
		Type: int32(cb.HeaderType_CONFIG),
	})
	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdr,
		},
		Data: []byte("bad data"),
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, &cb.ConfigEnvelope{})
	require.Error(t, err, "Expected error for malformed payload data")

	chdr, _ = proto.Marshal(&cb.ChannelHeader{
		Type: int32(cb.HeaderType_CONFIG),
	})
	configEnv, _ := proto.Marshal(&cb.ConfigEnvelope{})
	payload, _ = proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdr,
		},
		Data: configEnv,
	})
	env.Payload = payload
	_, err = UnmarshalEnvelopeOfType(env, cb.HeaderType_CONFIG, &cb.ConfigEnvelope{})
	require.NoError(t, err, "Unexpected error unmarshalling envelope")
}

func TestExtractEnvelopeNilData(t *testing.T) {
	block := &cb.Block{}
	_, err := ExtractEnvelope(block, 0)
	require.Error(t, err, "Nil data")
}

func TestExtractEnvelopeWrongIndex(t *testing.T) {
	block := testBlock()
	if _, err := ExtractEnvelope(block, len(block.GetData().Data)); err == nil {
		t.Fatal("Expected envelope extraction to fail (wrong index)")
	}
}

func TestExtractEnvelope(t *testing.T) {
	if envelope, err := ExtractEnvelope(testBlock(), 0); err != nil {
		t.Fatalf("Expected envelop extraction to succeed: %s", err)
	} else if !proto.Equal(envelope, testEnvelope()) {
		t.Fatal("Expected extracted envelope to match test envelope")
	}
}

func TestExtractPayload(t *testing.T) {
	if payload, err := UnmarshalPayload(testEnvelope().Payload); err != nil {
		t.Fatalf("Expected payload extraction to succeed: %s", err)
	} else if !proto.Equal(payload, testPayload()) {
		t.Fatal("Expected extracted payload to match test payload")
	}
}

// Helper functions

func testPayload() *cb.Payload {
	return &cb.Payload{
		Header: MakePayloadHeader(
			MakeChannelHeader(cb.HeaderType_MESSAGE, int32(1), "test", 0),
			MakeSignatureHeader([]byte("creator"), []byte("nonce"))),
		Data: []byte("test"),
	}
}

func testEnvelope() *cb.Envelope {
	// No need to set the signature
	return &cb.Envelope{Payload: MarshalOrPanic(testPayload())}
}

func testBlock() *cb.Block {
	// No need to set the block's Header, or Metadata
	return &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{MarshalOrPanic(testEnvelope())},
		},
	}
}

func TestGetRandomNonce(t *testing.T) {
	key1, err := getRandomNonce()
	require.NoErrorf(t, err, "error getting random bytes")
	require.Len(t, key1, NonceSize)
}
