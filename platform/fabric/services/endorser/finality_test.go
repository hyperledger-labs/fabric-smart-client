/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

// newFinalityFixture returns a receiveFixture (net1 default, net2 other, both with
// channel ch1 but different genesis hashes) wired with a Finality mock for each
// network's channel, so a test can tell which network a finality view queried.
func newFinalityFixture(t *testing.T) (f *receiveFixture, fin1, fin2 *mock.Finality) {
	t.Helper()
	f = newReceiveFixture(t, nil)
	ch1, err := f.nets[0].Channel("ch1")
	require.NoError(t, err)
	fin1 = &mock.Finality{}
	ch1.(*mock.Channel).FinalityReturns(fin1)
	fin2 = &mock.Finality{}
	f.ch2.FinalityReturns(fin2)
	return f, fin1, fin2
}

func TestFinalityViewFactory_JSON(t *testing.T) {
	t.Parallel()
	f, fin1, fin2 := newFinalityFixture(t)
	in, err := json.Marshal(&Finality{TxID: "tx1", Network: "net2", Channel: "ch1"})
	require.NoError(t, err)

	v, err := (&FinalityViewFactory{}).NewView(in)
	require.NoError(t, err)
	_, err = v.Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, fin2.IsFinalCallCount())
	require.Zero(t, fin1.IsFinalCallCount())
}

// JSON with leading whitespace is not JSON to the factory: its first byte could be the 0x0a
// of a FinalityRequest. It fails to unmarshal as the proto.
func TestFinalityViewFactory_JSONWithLeadingWhitespaceErrors(t *testing.T) {
	t.Parallel()
	in, err := json.Marshal(&Finality{TxID: "tx1", Network: "net2", Channel: "ch1"})
	require.NoError(t, err)
	in = append([]byte("  \n\t"), in...)

	_, err = (&FinalityViewFactory{}).NewView(in)
	require.Error(t, err)
}

func TestFinalityViewFactory_ProtoMatchingKeyPicksNetwork(t *testing.T) {
	t.Parallel()
	f, fin1, fin2 := newFinalityFixture(t)
	in, err := proto.Marshal(&protos.FinalityRequest{
		TxId:               "tx1",
		Platform:           "generic",
		ChannelGenesisHash: net2Key.GenesisHash,
		Channel:            "ch1",
	})
	require.NoError(t, err)

	v, err := (&FinalityViewFactory{}).NewView(in)
	require.NoError(t, err)
	_, err = v.Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, fin2.IsFinalCallCount())
	require.Zero(t, fin1.IsFinalCallCount())
}

func TestFinalityViewFactory_ProtoNoGenesisHashUsesDefaultNetwork(t *testing.T) {
	t.Parallel()
	f, fin1, _ := newFinalityFixture(t)
	in, err := proto.Marshal(&protos.FinalityRequest{TxId: "tx1", Channel: "ch1"})
	require.NoError(t, err)

	v, err := (&FinalityViewFactory{}).NewView(in)
	require.NoError(t, err)
	_, err = v.Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, fin1.IsFinalCallCount())
}

func TestFinalityViewFactory_ProtoUnknownKeyErrors(t *testing.T) {
	t.Parallel()
	f, _, _ := newFinalityFixture(t)
	in, err := proto.Marshal(&protos.FinalityRequest{
		TxId:               "tx1",
		Platform:           "generic",
		ChannelGenesisHash: []byte("other"),
		Channel:            "ch1",
	})
	require.NoError(t, err)

	v, err := (&FinalityViewFactory{}).NewView(in)
	require.NoError(t, err)
	_, err = v.Call(f.ctx)
	require.ErrorContains(t, err, "no local network matches channel [ch1]")
}

func TestFinalityViewFactory_ProtoUnspecifiedPlatformWithGenesisHashErrors(t *testing.T) {
	t.Parallel()
	in, err := proto.Marshal(&protos.FinalityRequest{
		TxId:               "tx1",
		ChannelGenesisHash: net1Key.GenesisHash,
		Channel:            "ch1",
	})
	require.NoError(t, err)

	_, err = (&FinalityViewFactory{}).NewView(in)
	require.Error(t, err)
}

// A FinalityRequest starts with 0x0a, the tag of tx_id, which is also '\n', and the next
// byte is the txID length: 9, 10, 13 and 32 are whitespace and 123 is '{'. Each such request
// decodes as the proto.
func TestFinalityViewFactory_ProtoWithAnyTxIDLength(t *testing.T) {
	t.Parallel()
	for _, n := range []int{9, 10, 13, 32, 64, 123} {
		// Digits: a txID of 9, 10, 13 or 32 digits alone is a valid JSON document
		// behind its tag and length byte, but not a JSON object.
		txID := strings.Repeat("1", n)
		for _, channel := range []string{"ch1", ""} {
			in, err := proto.Marshal(&protos.FinalityRequest{TxId: txID, Channel: channel})
			require.NoError(t, err)

			v, err := (&FinalityViewFactory{}).NewView(in)
			require.NoError(t, err, "txID length %d, channel [%s]", n, channel)
			require.Equal(t, txID, v.(*finalityView).TxID, "txID length %d, channel [%s]", n, channel)
			require.Equal(t, channel, v.(*finalityView).Channel, "txID length %d, channel [%s]", n, channel)
		}
	}

	// A txID chosen so that the request, 0a 7b followed by the txID, is a valid JSON object.
	txID := `"TxID":"` + strings.Repeat("a", 123-len(`"TxID":"`)-len(`"}`)) + `"}`
	require.Len(t, txID, 123)
	in, err := proto.Marshal(&protos.FinalityRequest{TxId: txID})
	require.NoError(t, err)
	require.True(t, json.Valid(in), "the request is a valid JSON object")
	v, err := (&FinalityViewFactory{}).NewView(in)
	require.NoError(t, err)
	require.Equal(t, txID, v.(*finalityView).TxID)
	require.Empty(t, v.(*finalityView).Network, "not decoded as the JSON Finality")
}

func TestFinalityViewFactory_EmptyInputErrors(t *testing.T) {
	t.Parallel()
	_, err := (&FinalityViewFactory{}).NewView(nil)
	require.Error(t, err)
}
