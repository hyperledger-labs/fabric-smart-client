/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	// logger is the ledger package logger.
	logger = logging.MustGetLogger()
)

// ledger implements the driver.Ledger interface for FabricX.
type ledger struct {
	// client is the BlockQueryServiceClient for interacting with the committer.
	client committerpb.BlockQueryServiceClient
	// queryClient is the QueryServiceClient for querying transaction status.
	queryClient committerpb.QueryServiceClient
	// baseCtx is the background context for RPC calls.
	baseCtx context.Context
}

// New returns a new ledger instance with the given clients and base context.
func New(client committerpb.BlockQueryServiceClient, queryClient committerpb.QueryServiceClient, baseCtx context.Context) *ledger {
	return &ledger{
		client:      client,
		queryClient: queryClient,
		baseCtx:     baseCtx,
	}
}

// GetLedgerInfo returns information about the ledger, such as height and current block hash.
func (c *ledger) GetLedgerInfo() (*driver.LedgerInfo, error) {
	info, err := c.client.GetBlockchainInfo(c.baseCtx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get blockchain info")
	}
	return &driver.LedgerInfo{
		Height:            info.Height,
		CurrentBlockHash:  info.CurrentBlockHash,
		PreviousBlockHash: info.PreviousBlockHash,
	}, nil
}

// GetTransactionByID returns the processed transaction for the given transaction ID.
func (c *ledger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	env, err := c.client.GetTxByID(c.baseCtx, &committerpb.TxID{TxId: txID})
	if err != nil {
		return nil, errors.Wrapf(finality.TxNotFound, "failed to get tx for txID [%s]: %s", txID, err)
	}

	res, err := c.queryClient.GetTransactionStatus(c.baseCtx, &committerpb.TxStatusQuery{
		TxIds: []string{txID},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get transaction status for txID [%s]", txID)
	}
	if len(res.Statuses) == 0 {
		return nil, errors.Errorf("no status returned for txID [%s]", txID)
	}

	results, err := unpackResults(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unpack results for txID [%s]", txID)
	}

	envRaw, err := protoutil.Marshal(env)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal envelope for txID [%s]", txID)
	}

	return &ProcessedTransaction{
		txID:           txID,
		results:        results,
		validationCode: int32(res.Statuses[0].Status),
		envelope:       envRaw,
	}, nil
}

// GetBlockNumberByTxID returns the block number that contains the given transaction ID.
func (c *ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	block, err := c.client.GetBlockByTxID(c.baseCtx, &committerpb.TxID{TxId: txID})
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get block for txID [%s]", txID)
	}
	return block.Header.Number, nil
}

// GetBlockByNumber returns the block at the given block number.
func (c *ledger) GetBlockByNumber(number uint64) (driver.Block, error) {
	block, err := c.client.GetBlockByNumber(c.baseCtx, &committerpb.BlockNumber{Number: number})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get block by number [%d]", number)
	}
	return &Block{Block: block}, nil
}

// Block wraps a Fabric block to provide ledger.Block functionality.
type Block struct {
	*cb.Block
}

// DataAt returns the data stored at the passed index within the block.
func (b *Block) DataAt(i int) []byte {
	return b.Data.Data[i]
}

// ProcessedTransaction returns the ProcessedTransaction at the passed index within the block.
func (b *Block) ProcessedTransaction(i int) (driver.ProcessedTransaction, error) {
	txRaw := b.Data.Data[i]
	env, _, chdr, err := fabricutils.UnmarshalTx(txRaw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal tx at index [%d]", i)
	}

	results, err := unpackResults(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unpack results at index [%d]", i)
	}

	return &ProcessedTransaction{
		txID:           chdr.TxId,
		results:        results,
		validationCode: int32(b.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER][i]),
		envelope:       txRaw,
	}, nil
}

// ProcessedTransaction implements the driver.ProcessedTransaction interface.
type ProcessedTransaction struct {
	txID           string
	results        []byte
	validationCode int32
	envelope       []byte
}

func NewProcessedTransaction(txID string, results []byte, validationCode int32, envelope []byte) *ProcessedTransaction {
	return &ProcessedTransaction{txID: txID, results: results, validationCode: validationCode, envelope: envelope}
}

// TxID returns the transaction ID.
func (t *ProcessedTransaction) TxID() string {
	return t.txID
}

// Results returns the transaction results.
func (t *ProcessedTransaction) Results() []byte {
	return t.results
}

// ValidationCode returns the validation code of the transaction.
func (t *ProcessedTransaction) ValidationCode() int32 {
	return t.validationCode
}

// IsValid returns true if the transaction was committed (validation code 0).
func (t *ProcessedTransaction) IsValid() bool {
	return t.validationCode == int32(committerpb.Status_COMMITTED)
}

// Envelope returns the raw transaction envelope.
func (t *ProcessedTransaction) Envelope() []byte {
	return t.envelope
}

// unpackResults extracts the payload data from a transaction payload.
// It returns the serialized read-write set (applicationpb.Tx) contained in the payload.
func unpackResults(payloadRaw []byte) ([]byte, error) {
	payl, err := protoutil.UnmarshalPayload(payloadRaw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal payload")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal channel header")
	}

	if cb.HeaderType(chdr.Type) != cb.HeaderType_MESSAGE {
		return nil, errors.Errorf("only HeaderType_MESSAGE Transactions are supported, provided type %d", chdr.Type)
	}

	// For FabricX, Payload.Data contains the serialized rwset (applicationpb.Tx)
	return payl.Data, nil
}
