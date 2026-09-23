/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

// malformed is a truncated varint: the continuation bit is set with no byte following,
// so every protobuf message rejects it.
var malformed = []byte{0xff}

// TestUnmarshalers checks each unmarshaler round-trips a populated message and reports
// malformed input as an error.
func TestUnmarshalers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		msg       proto.Message
		unmarshal func([]byte) (proto.Message, error)
	}{
		{
			name:      "Proposal",
			msg:       &peer.Proposal{Header: []byte("header"), Payload: []byte("payload")},
			unmarshal: func(b []byte) (proto.Message, error) { return protoutil.UnmarshalProposal(b) },
		},
		{
			name:      "SerializedIdentity",
			msg:       &msp.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("cert")},
			unmarshal: func(b []byte) (proto.Message, error) { return protoutil.UnmarshalSerializedIdentity(b) },
		},
		{
			name:      "ChaincodeInvocationSpec",
			msg:       &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{ChaincodeId: &peer.ChaincodeID{Name: "mycc"}}},
			unmarshal: func(b []byte) (proto.Message, error) { return protoutil.UnmarshalChaincodeInvocationSpec(b) },
		},
		{
			name:      "ChaincodeHeaderExtension",
			msg:       &peer.ChaincodeHeaderExtension{ChaincodeId: &peer.ChaincodeID{Name: "mycc"}},
			unmarshal: func(b []byte) (proto.Message, error) { return protoutil.UnmarshalChaincodeHeaderExtension(b) },
		},
		{
			name:      "ChaincodeEvent",
			msg:       &peer.ChaincodeEvent{ChaincodeId: "mycc", TxId: "tx1", EventName: "ev", Payload: []byte("data")},
			unmarshal: func(b []byte) (proto.Message, error) { return protoutil.UnmarshalChaincodeEvents(b) },
		},
		{
			name:      "ConfigEnvelope",
			msg:       &common.ConfigEnvelope{Config: &common.Config{Sequence: 1}},
			unmarshal: func(b []byte) (proto.Message, error) { return protoutil.UnmarshalConfigEnvelope(b) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := proto.Marshal(tc.msg)
			require.NoError(t, err)

			got, err := tc.unmarshal(raw)
			require.NoError(t, err)
			assert.True(t, proto.Equal(tc.msg, got), "round-trip mismatch: want %v, got %v", tc.msg, got)

			_, err = tc.unmarshal(malformed)
			assert.Error(t, err, "malformed input is reported")
		})
	}
}
