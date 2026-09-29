/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// TestUnpackEnvelopePayload_NilHeaderDoesNotPanic covers a payload with no Header field:
// UnpackEnvelopePayload must unmarshal the channel header via the nil-safe GetHeader()
// accessor instead of dereferencing a nil Header. A nil ChannelHeader decodes to the
// zero-value HeaderType (HeaderType_MESSAGE), so this is accepted rather than rejected.
func TestUnpackEnvelopePayload_NilHeaderDoesNotPanic(t *testing.T) {
	t.Parallel()

	payloadRaw, err := proto.Marshal(&common.Payload{Data: []byte("data")})
	require.NoError(t, err)

	var upe *UnpackedEnvelope
	require.NotPanics(t, func() {
		upe, _, err = UnpackEnvelopePayload(payloadRaw)
	})
	require.NoError(t, err)
	require.Equal(t, []byte("data"), upe.Results)
}

func TestNewEnvelopeClonesInputs(t *testing.T) {
	t.Parallel()

	nonce, creator, results := []byte("nonce"), []byte("creator"), []byte("results")
	env := NewEnvelope("tx1", nonce, creator, results, &common.Envelope{Payload: []byte("payload")})
	nonce[0], creator[0], results[0] = 'X', 'X', 'X'

	require.Equal(t, "tx1", env.TxID())
	require.Equal(t, []byte("nonce"), env.Nonce())
	require.Equal(t, []byte("creator"), env.Creator())
	require.Equal(t, []byte("results"), env.Results())
	require.Equal(t, []byte("payload"), env.Envelope().Payload)
}

func TestEnvelopeBytesRoundTrip(t *testing.T) {
	t.Parallel()

	env := NewEnvelope("tx1", nil, nil, nil, &common.Envelope{Payload: []byte("payload"), Signature: []byte("sig")})
	raw, err := env.Bytes()
	require.NoError(t, err)

	decoded := NewEmptyEnvelope()
	require.NoError(t, decoded.FromBytes(raw))
	require.True(t, proto.Equal(env.Envelope(), decoded.Envelope()))

	require.Error(t, NewEmptyEnvelope().FromBytes([]byte("garbage")))
}

func TestEnvelopeString(t *testing.T) {
	t.Parallel()

	s := NewEnvelope("tx1", nil, nil, nil, &common.Envelope{Payload: []byte("payload")}).String()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &decoded))
	require.Contains(t, decoded, "payload")
}

func TestUnpackEnvelopePayload(t *testing.T) {
	t.Parallel()

	marshal := func(p *common.Payload) []byte {
		raw, err := proto.Marshal(p)
		require.NoError(t, err)
		return raw
	}
	chdr := func(typ common.HeaderType) []byte {
		raw, err := proto.Marshal(&common.ChannelHeader{Type: int32(typ), TxId: "tx1"})
		require.NoError(t, err)
		return raw
	}

	t.Run("message", func(t *testing.T) {
		t.Parallel()
		env := &common.Envelope{Payload: marshal(&common.Payload{
			Header: &common.Header{ChannelHeader: chdr(common.HeaderType_MESSAGE)},
			Data:   []byte("data"),
		})}
		envRaw, err := proto.Marshal(env)
		require.NoError(t, err)

		upe, typ, err := UnpackEnvelopeFromBytes(envRaw)
		require.NoError(t, err)
		require.Equal(t, int32(common.HeaderType_MESSAGE), typ)
		require.Equal(t, "tx1", upe.ID())
		require.Equal(t, []byte("data"), upe.Results)
	})

	t.Run("invalid payload", func(t *testing.T) {
		t.Parallel()
		_, typ, err := UnpackEnvelopePayload([]byte("garbage"))
		require.ErrorContains(t, err, "failed to unmarshal payload")
		require.Equal(t, int32(-1), typ)
	})

	t.Run("invalid channel header", func(t *testing.T) {
		t.Parallel()
		_, typ, err := UnpackEnvelopePayload(marshal(&common.Payload{
			Header: &common.Header{ChannelHeader: []byte("garbage")},
		}))
		require.ErrorContains(t, err, "failed to unmarshal channel header")
		require.Equal(t, int32(-1), typ)
	})

	t.Run("rejects non-message header type", func(t *testing.T) {
		t.Parallel()
		_, typ, err := UnpackEnvelopePayload(marshal(&common.Payload{
			Header: &common.Header{ChannelHeader: chdr(common.HeaderType_ENDORSER_TRANSACTION)},
		}))
		require.ErrorContains(t, err, "only HeaderType_MESSAGE Transactions are supported")
		require.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), typ)
	})

	t.Run("invalid envelope bytes", func(t *testing.T) {
		t.Parallel()
		_, typ, err := UnpackEnvelopeFromBytes([]byte("garbage"))
		require.Error(t, err)
		require.Equal(t, int32(-1), typ)
	})
}
