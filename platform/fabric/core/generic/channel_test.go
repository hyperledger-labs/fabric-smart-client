/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	xprotoutil "github.com/hyperledger/fabric-x-common/protoutil"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// blockLedger serves block 0 as block, or fails with err.
type blockLedger struct {
	driver.Ledger
	block *common.Block
	err   error
	calls int
}

// block0 is a driver.Block that exposes the header of the block it wraps, as the
// ledgers of both platforms do.
type block0 struct {
	driver.Block
	b *common.Block
}

func (b *block0) GetHeader() *common.BlockHeader { return b.b.GetHeader() }

func (l *blockLedger) GetBlockByNumber(number uint64) (driver.Block, error) {
	l.calls++
	if number != 0 {
		return nil, errors.Errorf("unexpected block [%d]", number)
	}
	if l.err != nil {
		return nil, l.err
	}
	return &block0{b: l.block}, nil
}

func TestChannelKey(t *testing.T) {
	t.Parallel()
	header := &common.BlockHeader{Number: 0, DataHash: []byte("data")}
	ledger := &blockLedger{err: errors.New("unavailable")}
	c := &Channel{ChannelName: "ch1", Platform: "generic", LedgerService: ledger}

	// A failed read is not cached.
	_, err := c.ChannelKey()
	require.ErrorContains(t, err, "failed reading block 0 of channel [ch1]: unavailable")

	ledger.err = nil
	ledger.block = &common.Block{Header: header}
	key, err := c.ChannelKey()
	require.NoError(t, err)
	require.Equal(t, &driver.ChannelKey{Platform: "generic", Channel: "ch1", GenesisHash: xprotoutil.BlockHeaderHash(header)}, key)

	// The key is read once.
	_, err = c.ChannelKey()
	require.NoError(t, err)
	require.Equal(t, 2, ledger.calls)
}

func TestChannelKey_NoHeader(t *testing.T) {
	t.Parallel()
	c := &Channel{ChannelName: "ch1", LedgerService: &blockLedger{block: &common.Block{}}}

	_, err := c.ChannelKey()
	require.ErrorContains(t, err, "block 0 of channel [ch1] has no header")
}
