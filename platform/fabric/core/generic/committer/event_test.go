/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/test-go/testify/require"
)

func TestEvent(t *testing.T) {
	t.Run("validChaincodeEvent", func(t *testing.T) {
		t.Run("Returns true for valid chaincode event ", func(t *testing.T) {
			event := &peer.ChaincodeEvent{
				ChaincodeId: "1",
				TxId:        "TXNID1",
				EventName:   "EVENT_1",
				Payload:     []byte("PAYLOAD_1"),
			}

			validity := validChaincodeEvent(event)
			require.Equal(t, validity, true, "Valid chaincode Event")
		})

		t.Run("Returns false for empty transaction ID", func(t *testing.T) {
			event := &peer.ChaincodeEvent{
				ChaincodeId: "1",
				TxId:        "",
				EventName:   "EVENT_1",
				Payload:     []byte("PAYLOAD_1"),
			}

			validity := validChaincodeEvent(event)
			require.Equal(t, validity, false, "Invalid transaction ID")
		})

		t.Run("Returns false for empty event name", func(t *testing.T) {
			event := &peer.ChaincodeEvent{
				ChaincodeId: "1",
				TxId:        "TXNID1",
				EventName:   "",
				Payload:     []byte("PAYLOAD_1"),
			}

			validity := validChaincodeEvent(event)
			require.Equal(t, validity, false, "Invalid event name")
		})

		t.Run("Returns false for empty chaincode ID", func(t *testing.T) {
			event := &peer.ChaincodeEvent{
				ChaincodeId: "",
				TxId:        "TXNID1",
				EventName:   "EVENT_1",
				Payload:     []byte("PAYLOAD_1"),
			}

			validity := validChaincodeEvent(event)
			require.Equal(t, validity, false, "Invalid chaincode ID")
		})
	})
}
