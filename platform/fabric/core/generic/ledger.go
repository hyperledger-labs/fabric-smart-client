/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
)

type Ledger struct {
	ChannelName      string
	ChaincodeManager driver.ChaincodeManager
	LocalMembership  driver.LocalMembership
	ConfigService    driver.ConfigService
}

func NewLedger(
	channelName string,
	chaincodeManager driver.ChaincodeManager,
	localMembership driver.LocalMembership,
	configService driver.ConfigService,
) *Ledger {
	return &Ledger{ChannelName: channelName, ChaincodeManager: chaincodeManager, LocalMembership: localMembership, ConfigService: configService}
}

func (c *Ledger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	pt := &peer.ProcessedTransaction{}
	if err := c.queryChaincode(GetTransactionByID, txID, pt); err != nil {
		return nil, err
	}
	return transaction.NewProcessedTransaction(pt)
}

func (c *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	block := &common.Block{}
	if err := c.queryChaincode(GetBlockByTxID, txID, block); err != nil {
		return 0, err
	}
	return block.Header.Number, nil
}

// GetBlockByNumber fetches a block by number
func (c *Ledger) GetBlockByNumber(number uint64) (driver.Block, error) {
	block := &common.Block{}
	if err := c.queryChaincode(GetBlockByNumber, number, block); err != nil {
		return nil, err
	}
	return &Block{Block: block}, nil
}

func (c *Ledger) queryChaincode(function string, param any, result proto.Message) error {
	raw, err := c.ChaincodeManager.Chaincode("qscc").
		NewInvocation(function, c.ChannelName, param).
		WithSignerIdentity(c.LocalMembership.DefaultIdentity()).
		WithEndorsersByConnConfig(c.ConfigService.PickPeer(driver.PeerForQuery)).
		Query()
	if err != nil {
		return errors.Wrap(err, "query chaincode failed")
	}

	if err := proto.Unmarshal(raw, result); err != nil {
		return errors.Wrap(err, "unmashal failed")
	}
	return nil
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
	return transaction.NewProcessedTransaction(&peer.ProcessedTransaction{
		TransactionEnvelope: env,
		ValidationCode:      int32(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER][i]),
	})
}
