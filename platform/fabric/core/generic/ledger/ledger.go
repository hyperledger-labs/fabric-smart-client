/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
)

const (
	QuerySystemChaincode = "qscc"

	GetBlockByNumber   string = "GetBlockByNumber"
	GetTransactionByID string = "GetTransactionByID"
	GetBlockByTxID     string = "GetBlockByTxID"
)

type Ledger struct {
	ChannelName        string
	ChaincodeManager   driver.ChaincodeManager
	LocalMembership    driver.LocalMembership
	ConfigService      driver.ConfigService
	TransactionManager driver.TransactionManager
}

func New(
	channelName string,
	chaincodeManager driver.ChaincodeManager,
	localMembership driver.LocalMembership,
	configService driver.ConfigService,
	transactionManager driver.TransactionManager,
) driver.Ledger {
	return &Ledger{
		ChannelName:        channelName,
		ChaincodeManager:   chaincodeManager,
		LocalMembership:    localMembership,
		ConfigService:      configService,
		TransactionManager: transactionManager,
	}
}

func (c *Ledger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	raw, err := c.queryChaincode(GetTransactionByID, txID)
	if err != nil {
		return nil, err
	}
	return c.TransactionManager.NewProcessedTransaction(raw)
}

func (c *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	raw, err := c.queryChaincode(GetBlockByTxID, txID)
	if err != nil {
		return 0, err
	}
	block := &common.Block{}
	if err := proto.Unmarshal(raw, block); err != nil {
		return 0, errors.Wrap(err, "unmarshal failed")
	}
	return block.Header.Number, nil
}

// GetBlockByNumber fetches a block by number
func (c *Ledger) GetBlockByNumber(number uint64) (driver.Block, error) {
	raw, err := c.queryChaincode(GetBlockByNumber, number)
	if err != nil {
		return nil, err
	}
	block := &common.Block{}
	if err := proto.Unmarshal(raw, block); err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}
	return &Block{Block: block, TransactionManager: c.TransactionManager}, nil
}

func (c *Ledger) queryChaincode(function string, param any) ([]byte, error) {
	return c.ChaincodeManager.Chaincode(QuerySystemChaincode).
		NewInvocation(function, c.ChannelName, param).
		WithSignerIdentity(c.LocalMembership.DefaultIdentity()).
		WithEndorsersByConnConfig(c.ConfigService.PickPeer(driver.PeerForQuery)).
		Query()
}

// Block wraps a Fabric block
type Block struct {
	*common.Block
	TransactionManager driver.TransactionManager
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
	pt := &peer.ProcessedTransaction{
		TransactionEnvelope: env,
		ValidationCode:      int32(b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER][i]),
	}
	ptRaw, err := proto.Marshal(pt)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}
	return b.TransactionManager.NewProcessedTransaction(ptRaw)
}
