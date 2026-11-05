/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

const (
	QuerySystemChaincode = "qscc"

	GetChainInfo       string = "GetChainInfo"
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
) *Ledger {
	return &Ledger{
		ChannelName:        channelName,
		ChaincodeManager:   chaincodeManager,
		LocalMembership:    localMembership,
		ConfigService:      configService,
		TransactionManager: transactionManager,
	}
}

func (c *Ledger) GetLedgerInfo() (*driver.LedgerInfo, error) {
	raw, err := c.queryChaincode(GetChainInfo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed querying chain info")
	}
	bi := &common.BlockchainInfo{}
	if err := proto.Unmarshal(raw, bi); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling block info")
	}
	return &driver.LedgerInfo{
		Height:            bi.Height,
		CurrentBlockHash:  bi.CurrentBlockHash,
		PreviousBlockHash: bi.PreviousBlockHash,
	}, nil
}

// GetTransactionByID returns the transaction with the given txID and its
// validation code. qscc returns the first envelope carrying txID, which can be
// an invalid envelope forged with the txID and ordered ahead of the honest one
// in the same block. When that envelope is invalid, GetTransactionByID returns
// the valid endorser transaction with the same txID in its block, if there is
// one (see committer.ShadowedEnvelopes), and otherwise the envelope qscc
// returned. It asks the same peer for the block, and returns an error, not the
// invalid envelope, when it cannot read the block.
func (c *Ledger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	peerConnConf, err := c.pickQueryPeer()
	if err != nil {
		return nil, err
	}
	raw, err := c.queryPeer(peerConnConf, GetTransactionByID, txID)
	if err != nil {
		return nil, err
	}
	// NewProcessedTransaction rejects an envelope that is not an endorser
	// transaction, which an invalid forgery can be; its error is returned
	// only if no valid envelope shadows it.
	pt, ptErr := c.TransactionManager.NewProcessedTransaction(raw)
	if ptErr == nil && pt.IsValid() {
		return pt, nil
	}
	if ptErr != nil {
		code := &peer.ProcessedTransaction{}
		if err := proto.Unmarshal(raw, code); err != nil || code.GetValidationCode() == int32(peer.TxValidationCode_VALID) {
			return nil, ptErr
		}
	}
	shadowing, err := c.shadowingTransaction(peerConnConf, txID)
	if err != nil {
		return nil, err
	}
	if shadowing != nil {
		return shadowing, nil
	}
	return pt, ptErr
}

// shadowingTransaction returns the valid endorser transaction carrying txID in
// the block the peer holds for txID, or nil if the block has none. This is the
// rule of committer.ShadowedEnvelopes: only a valid endorser transaction
// shadows. Only that block needs checking: Fabric flags an envelope carrying
// txID in a later block DUPLICATE_TXID.
func (c *Ledger) shadowingTransaction(peerConnConf *grpc.ConnectionConfig, txID string) (driver.ProcessedTransaction, error) {
	raw, err := c.queryPeer(peerConnConf, GetBlockByTxID, txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed fetching block of transaction [%s]", txID)
	}
	block := &common.Block{}
	if err := proto.Unmarshal(raw, block); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling block of transaction [%s]", txID)
	}
	var flags []byte
	if metadata := block.GetMetadata().GetMetadata(); len(metadata) > int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		flags = metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]
	}
	for i, tx := range block.GetData().GetData() {
		if i >= len(flags) || peer.TxValidationCode(flags[i]) != peer.TxValidationCode_VALID {
			continue
		}
		_, _, chdr, err := fabricutils.UnmarshalTx(tx)
		if err != nil || chdr.GetTxId() != txID || common.HeaderType(chdr.GetType()) != common.HeaderType_ENDORSER_TRANSACTION {
			continue
		}
		return (&Block{Block: block, TransactionManager: c.TransactionManager}).ProcessedTransaction(i)
	}
	return nil, nil
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
	// A block that unmarshals successfully can still carry no Header, since
	// an absent optional message is a valid encoding. The bytes come from a
	// remote peer, so treat a missing header as a query failure rather than
	// dereferencing block.Header directly.
	header := block.GetHeader()
	if header == nil {
		return 0, errors.Errorf("block for txID [%s] has no header", txID)
	}
	return header.Number, nil
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
	peerConnConf, err := c.pickQueryPeer()
	if err != nil {
		return nil, err
	}
	return c.queryPeer(peerConnConf, function, param)
}

func (c *Ledger) pickQueryPeer() (*grpc.ConnectionConfig, error) {
	peerConnConf := c.ConfigService.PickPeer(driver.PeerForQuery)
	if peerConnConf == nil {
		return nil, errors.New("no peer configured for query")
	}
	return peerConnConf, nil
}

func (c *Ledger) queryPeer(peerConnConf *grpc.ConnectionConfig, function string, param any) ([]byte, error) {
	params := []any{c.ChannelName}
	if param != nil {
		params = append(params, param)
	}
	return c.ChaincodeManager.Chaincode(QuerySystemChaincode).
		NewInvocation(function, params...).
		WithSignerIdentity(c.LocalMembership.DefaultIdentity()).
		WithEndorsersByConnConfig(peerConnConf).
		Query()
}

// Block wraps a Fabric block
type Block struct {
	*common.Block
	TransactionManager driver.TransactionManager
}

// DataAt returns the data stored at the passed index. block.Data is
// populated by whoever produced the block, so a malformed or malicious
// block (e.g. fetched via GetBlockByNumber from a remote peer) can have a
// nil Data or a Data.Data shorter than the caller expects, which would
// otherwise panic on a bare slice index.
func (b *Block) DataAt(i int) []byte {
	if b.Data == nil || i < 0 || i >= len(b.Data.Data) {
		return nil
	}
	return b.Data.Data[i]
}

// ProcessedTransaction returns the ProcessedTransaction at passed index
func (b *Block) ProcessedTransaction(i int) (driver.ProcessedTransaction, error) {
	if b.Data == nil || i < 0 || i >= len(b.Data.Data) {
		return nil, errors.Errorf("transaction index [%d] out of range [0,%d)", i, len(b.Data.GetData()))
	}
	if b.Metadata == nil || int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) >= len(b.Metadata.Metadata) {
		return nil, errors.New("block metadata missing transactions filter entry")
	}
	txFilter := b.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]
	if i >= len(txFilter) {
		return nil, errors.Errorf("transactions filter index [%d] out of range [0,%d)", i, len(txFilter))
	}

	env := &common.Envelope{}
	if err := proto.Unmarshal(b.Data.Data[i], env); err != nil {
		return nil, err
	}
	pt := &peer.ProcessedTransaction{
		TransactionEnvelope: env,
		ValidationCode:      int32(txFilter[i]),
	}
	ptRaw, err := proto.Marshal(pt)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}
	return b.TransactionManager.NewProcessedTransaction(ptRaw)
}
