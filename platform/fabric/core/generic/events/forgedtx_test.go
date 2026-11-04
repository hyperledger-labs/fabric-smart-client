/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import (
	"context"
	"strconv"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// TestOnBlockForgedEnvelope runs blocks holding several envelopes with the same
// txID through the listener manager. With a valid envelope in the block, a
// listener on the txID receives that envelope's event only, and the cache keeps
// it. Without one, the listener first receives the first envelope's event.
func TestOnBlockForgedEnvelope(t *testing.T) {
	t.Parallel()
	const (
		valid  = peer.TxValidationCode_VALID
		forged = peer.TxValidationCode_BAD_PROPOSAL_TXID
	)
	chdr, err := proto.Marshal(&common.ChannelHeader{TxId: "tx1", Type: int32(common.HeaderType_ENDORSER_TRANSACTION)})
	require.NoError(t, err)
	payload, err := proto.Marshal(&common.Payload{Header: &common.Header{ChannelHeader: chdr}})
	require.NoError(t, err)
	envelope, err := proto.Marshal(&common.Envelope{Payload: payload})
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		codes    []peer.TxValidationCode
		want     int  // index of the envelope whose event the listener receives first
		onlyOnce bool // the listener receives no other event
	}{
		"forged then honest":     {[]peer.TxValidationCode{forged, valid}, 1, true},
		"honest then duplicate":  {[]peer.TxValidationCode{valid, peer.TxValidationCode_DUPLICATE_TXID}, 0, true},
		"forged, honest, forged": {[]peer.TxValidationCode{forged, valid, forged}, 1, true},
		"invalid only":           {[]peer.TxValidationCode{forged, peer.TxValidationCode_MVCC_READ_CONFLICT}, 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			flags := make([]byte, len(tc.codes))
			data := make([][]byte, len(tc.codes))
			eventsByTx := make([]map[driver.Namespace]testEvent, len(tc.codes))
			for i, code := range tc.codes {
				flags[i] = uint8(code)
				data[i] = envelope
				// The status names the envelope the event is mapped from.
				eventsByTx[i] = map[driver.Namespace]testEvent{"ns": {id: "tx1", status: strconv.Itoa(i)}}
			}
			block := &common.Block{
				Header:   &common.BlockHeader{Number: testBlockNumber},
				Data:     &common.BlockData{Data: data},
				Metadata: &common.BlockMetadata{Metadata: [][]byte{nil, nil, flags}},
			}

			manager := newTestListenerManager(t)
			manager.mapper = &parallelBlockMapper[testEvent]{logger: manager.logger, cap: 4, mapper: staticMapper{eventsByTx: eventsByTx}}
			listener := newTestListener("l")
			require.NoError(t, manager.AddEventListener("tx1", listener))

			require.NoError(t, manager.onBlock(context.Background(), block))
			want := testEvent{id: "tx1", status: strconv.Itoa(tc.want)}
			require.Equal(t, want, waitForEvent(t, listener.calls))
			if tc.onlyOnce {
				require.Empty(t, listener.calls)
				cached, ok := manager.events.Get("tx1")
				require.True(t, ok)
				require.Equal(t, want, cached)
			}
		})
	}
}
