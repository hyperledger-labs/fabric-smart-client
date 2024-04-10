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
	raw, err := c.ChaincodeManager.Chaincode("qscc").NewInvocation(GetTransactionByID, c.ChannelName, txID).WithSignerIdentity(
		c.LocalMembership.DefaultIdentity(),
	).WithEndorsersByConnConfig(c.ConfigService.PickPeer(driver.PeerForQuery)).Query()
	if err != nil {
		return nil, err
	}

	logger.Debugf("got transaction by id [%s] of len [%d]", txID, len(raw))

	pt := &peer.ProcessedTransaction{}
	err = proto.Unmarshal(raw, pt)
	if err != nil {
		return nil, err
	}
	return newProcessedTransaction(pt)
}

func (c *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	res, err := c.ChaincodeManager.Chaincode("qscc").NewInvocation(GetBlockByTxID, c.ChannelName, txID).WithSignerIdentity(
		c.LocalMembership.DefaultIdentity(),
	).WithEndorsersByConnConfig(c.ConfigService.PickPeer(driver.PeerForQuery)).Query()
	if err != nil {
		return 0, err
	}

	block := &common.Block{}
	err = proto.Unmarshal(res, block)
	if err != nil {
		return 0, err
	}
	return block.Header.Number, nil
}

// GetBlockByNumber fetches a block by number
func (c *Ledger) GetBlockByNumber(number uint64) (driver.Block, error) {
	res, err := c.ChaincodeManager.Chaincode("qscc").NewInvocation(GetBlockByNumber, c.ChannelName, number).WithSignerIdentity(
		c.LocalMembership.DefaultIdentity(),
	).WithEndorsersByConnConfig(c.ConfigService.PickPeer(driver.PeerForQuery)).Query()
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
