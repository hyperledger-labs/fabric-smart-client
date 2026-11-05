/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protos_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
)

// FuzzJSONFromEnvelope fuzzes protos.JSONFromEnvelope, which reads transaction
// envelopes another node sends. It either fails or returns JSON that converts back
// to an envelope for the envelope's platform.
func FuzzJSONFromEnvelope(f *testing.F) {
	n := newFabricNode(f)
	n.channel.ChannelKeyReturns(testKey, nil)
	w, _ := n.endorsedTx(f)
	raw, err := w.Bytes()
	require.NoError(f, err)
	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(f, err)
	flat, err := protos.EnvelopeFromJSONFor([]byte(`{"TTxID":"tx","TChannel":"ch"}`), testFlat)
	require.NoError(f, err)

	f.Add(env)
	f.Add(flat)
	f.Add([]byte(nil))
	f.Add([]byte{0x0a, 0x07, 'g', 'e', 'n', 'e', 'r', 'i', 'c'})
	f.Add([]byte{0x0a, 0x07, 'g', 'e', 'n', 'e', 'r', 'i', 'c', 0x2a, 0x02, 0x22, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := protos.JSONFromEnvelope(data)
		if err != nil {
			return
		}
		msg := &protos.TransactionEnvelope{}
		require.NoError(t, proto.Unmarshal(data, msg))
		_, err = protos.EnvelopeFromJSONFor(out, msg.GetPlatform())
		require.NoError(t, err)
	})
}
