/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

// NewRWSet returns a RWSet for this ledger.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Channel) NewRWSet(txid string) (driver.RWSet, error) {
	return c.Vault.NewRWSet(txid)
}

// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Channel) GetRWSet(txid string, rwset []byte) (driver.RWSet, error) {
	return c.Vault.GetRWSet(txid, rwset)
}

// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
func (c *Channel) GetEphemeralRWSet(rwset []byte, namespaces ...string) (driver.RWSet, error) {
	return c.Vault.InspectRWSet(rwset, namespaces...)
}

// NewQueryExecutor gives handle to a query executor.
// A client can obtain more than one 'QueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (c *Channel) NewQueryExecutor() (driver.QueryExecutor, error) {
	return c.Vault.NewQueryExecutor()
}

// GetBlockByNumber fetches a block by number
func (c *Channel) GetBlockByNumber(number uint64) (driver.Block, error) {
	res, err := c.Chaincode("qscc").NewInvocation(GetBlockByNumber, c.ChannelName, number).WithSignerIdentity(
		c.Network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.Network.PickPeer(driver.PeerForQuery)).Query()
	if err != nil {
		return nil, err
	}

	b, err := protoutil.UnmarshalBlock(res)
	if err != nil {
		return nil, err
	}
	return &Block{Block: b}, nil
}

// Block wraps a Fabric block
type Block struct {
	*common.Block
}

// DataAt returns the data stored at the passed index
func (b *Block) DataAt(i int) []byte {
	return b.Data.Data[i]
}

// ProcessedTransaction returns the ProcessedTransaction at passed index
func (b *Block) ProcessedTransaction(i int) (driver.ProcessedTransaction, error) {
	env := &common.Envelope{}
	if err := proto.Unmarshal(b.Data.Data[i], env); err != nil {
		return nil, err
	}
	return newProcessedTransaction(&peer.ProcessedTransaction{
		TransactionEnvelope: env,
		ValidationCode:      int32(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER][i]),
	})
}
