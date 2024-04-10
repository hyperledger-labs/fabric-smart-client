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

// GetBlockByNumber fetches a block by number
func (c *Channel) GetBlockByNumber(number uint64) (driver.Block, error) {
	res, err := c.Chaincode("qscc").NewInvocation(GetBlockByNumber, c.ChannelName, number).WithSignerIdentity(
		c.Network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.Network.ConfigService().PickPeer(driver.PeerForQuery)).Query()
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
