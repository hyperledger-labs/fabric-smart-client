/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// NewRWSet returns a RWSet for this ledger.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *channel) NewRWSet(txid string) (driver.RWSet, error) {
	return c.vault.NewRWSet(txid)
}

// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *channel) GetRWSet(txid string, rwset []byte) (driver.RWSet, error) {
	return c.vault.GetRWSet(txid, rwset)
}

// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
func (c *channel) GetEphemeralRWSet(rwset []byte) (driver.RWSet, error) {
	return c.vault.InspectRWSet(rwset)
}

// NewQueryExecutor gives handle to a query executor.
// A client can obtain more than one 'QueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (c *channel) NewQueryExecutor() (driver.QueryExecutor, error) {
	return c.vault.NewQueryExecutor()
}

// GetBlockByNumber fetches a block by number
func (c *channel) GetBlockByNumber(number uint64) (driver.Block, error) {
	res, err := c.Chaincode("qscc").NewInvocation(driver.ChaincodeQuery, GetBlockByNumber, c.name, number).WithSignerIdentity(
		c.network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.network.Peers()...).Call()
	if err != nil {
		return nil, err
	}

	b, err := protoutil.UnmarshalBlock(res.([]byte))
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
