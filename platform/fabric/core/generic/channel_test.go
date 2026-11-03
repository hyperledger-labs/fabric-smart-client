/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	xprotoutil "github.com/hyperledger/fabric-x-common/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// blockLedger serves block 0 as block, or fails with err. A non-nil wait blocks
// every read until it is closed.
type blockLedger struct {
	driver.Ledger
	mu    sync.Mutex
	block *common.Block
	err   error
	calls int
	wait  chan struct{}
}

func (l *blockLedger) set(block *common.Block, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.block, l.err = block, err
}

func (l *blockLedger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// block0 is a driver.Block that exposes the header of the block it wraps, as the
// ledgers of both platforms do.
type block0 struct {
	driver.Block
	b *common.Block
}

func (b *block0) GetHeader() *common.BlockHeader { return b.b.GetHeader() }

func (l *blockLedger) GetBlockByNumber(number uint64) (driver.Block, error) {
	if l.wait != nil {
		<-l.wait
	}
	l.mu.Lock()
	defer l.mu.Unlock()
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

	_, err := c.ChannelKey()
	require.ErrorContains(t, err, "failed reading block 0 of channel [ch1]: unavailable")

	// The failure is reported without another read until the retry time.
	ledger.set(&common.Block{Header: header}, nil)
	_, err = c.ChannelKey()
	require.ErrorContains(t, err, "unavailable")
	require.Equal(t, 1, ledger.count())

	c.keyMu.Lock()
	c.keyRetryAt = time.Time{}
	c.keyMu.Unlock()
	key, err := c.ChannelKey()
	require.NoError(t, err)
	require.Equal(t, &driver.ChannelKey{Platform: "generic", Channel: "ch1", GenesisHash: xprotoutil.BlockHeaderHash(header)}, key)

	// The key is read once, and callers get a copy.
	key.GenesisHash[0] ^= 0xff
	again, err := c.ChannelKey()
	require.NoError(t, err)
	require.Equal(t, xprotoutil.BlockHeaderHash(header), again.GenesisHash)
	require.Equal(t, 2, ledger.count())
}

// A read that does not return in time fails every waiting caller, and concurrent
// callers share one read.
func TestChannelKey_Timeout(t *testing.T) {
	t.Parallel()
	ledger := &blockLedger{block: &common.Block{Header: &common.BlockHeader{}}, wait: make(chan struct{})}
	c := &Channel{ChannelName: "ch1", LedgerService: ledger, keyTimeout: 50 * time.Millisecond}

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			_, err := c.ChannelKey()
			assert.ErrorContains(t, err, "timed out reading block 0 of channel [ch1]")
		})
	}
	wg.Wait()

	// The read completes later and its key is kept.
	close(ledger.wait)
	require.Eventually(t, func() bool { _, err := c.ChannelKey(); return err == nil }, time.Second, 10*time.Millisecond)
	require.Equal(t, 1, ledger.count())
}

func TestChannelKey_NoHeader(t *testing.T) {
	t.Parallel()
	c := &Channel{ChannelName: "ch1", LedgerService: &blockLedger{block: &common.Block{}}}

	_, err := c.ChannelKey()
	require.ErrorContains(t, err, "block 0 of channel [ch1] has no header")
}
