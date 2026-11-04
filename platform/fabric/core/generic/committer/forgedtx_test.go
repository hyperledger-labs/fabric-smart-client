/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"bytes"
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commoncommitter "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// TestForgedEnvelopeDoesNotDiscardValidOne commits a block holding two
// envelopes with the same txID, on a node that does not know the transaction
// but tracks its namespace. The first is a forgery Fabric flags
// BAD_PROPOSAL_TXID; the second is the honest one, which Fabric finds VALID
// because it records a txID for duplicate detection only when its envelope is
// valid. The honest transaction commits and the forged envelope is never
// stored.
func TestForgedEnvelopeDoesNotDiscardValidOne(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	const txID = "tx1"
	forged := buildEnvelopeWithChaincodeEvent(t, &peer.ChaincodeEvent{TxId: txID, EventName: "forged"})
	honest := buildEnvelopeWithChaincodeEvent(t, &peer.ChaincodeEvent{TxId: txID, EventName: "honest"})

	// The vault and the envelope store keep their state across the two
	// envelopes and behave like the real ones: the store rejects a different
	// envelope under a stored txID, and discarding a transaction drops its
	// loaded read-write set. A read-write set's results are the bytes of the
	// envelope it was loaded from, so Match compares envelopes.
	status := map[string]fdriver.ValidationCode{}
	stored := map[string][]byte{}
	loaded := map[string][]byte{}
	c := &Committer{
		logger:            logger,
		ChannelConfig:     &fake.ChannelConfig{IDValue: "ch"},
		ConfigService:     &fake.ConfigService{NetworkNameValue: "fabric"},
		ProcessNamespaces: []string{"tracked-ns"},
		Vault: &fake.Vault{
			StatusFn: func(_ context.Context, id cdriver.TxID) (fdriver.ValidationCode, string, error) {
				if vc, ok := status[id]; ok {
					return vc, "", nil
				}
				return fdriver.Unknown, "", nil
			},
			SetDiscardedFn: func(_ context.Context, id cdriver.TxID, _ string) error {
				status[id] = fdriver.Invalid
				return nil
			},
			DiscardTxFn: func(_ context.Context, id cdriver.TxID, _ string) error {
				delete(loaded, id)
				status[id] = fdriver.Invalid
				return nil
			},
			RWSExistsFn: func(_ context.Context, id cdriver.TxID) bool { return loaded[id] != nil },
			MatchFn: func(_ context.Context, id cdriver.TxID, results []byte) error {
				if !bytes.Equal(loaded[id], results) {
					return errors.New("rwsets differ")
				}
				return nil
			},
			CommitTxFn: func(_ context.Context, id cdriver.TxID, _ cdriver.BlockNum, _ cdriver.TxNum) error {
				if loaded[id] == nil {
					return errors.Errorf("read-write set for [%s] could not be found", id)
				}
				status[id] = fdriver.Valid
				return nil
			},
		},
		EnvelopeService: &fake.EnvelopeService{
			ExistsFn: func(_ context.Context, id string) bool { return stored[id] != nil },
			StoreEnvelopeFn: func(_ context.Context, id string, env any) error {
				if old := stored[id]; old != nil && !bytes.Equal(old, env.([]byte)) {
					return errors.Wrapf(dbdriver.ErrConflict, "envelope [%s]", id)
				}
				stored[id] = env.([]byte)
				return nil
			},
		},
		RWSetLoaderService: &fake.RWSetLoader{
			InspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return &fake.RWSet{NamespacesList: []cdriver.Namespace{"tracked-ns"}}, nil, nil
			},
			GetFromEnvelopeFn: func(_ context.Context, id cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				loaded[id] = stored[id]
				return &fake.RWSet{}, nil, nil
			},
		},
		TransactionManager: &fake.TransactionManager{
			NewProcessedFromPayloadFn: func(payload []byte) (fdriver.ProcessedTransaction, int32, error) {
				return &fake.ProcessedTransaction{ResultsBytes: mustMarshalProto(t, &common.Envelope{Payload: payload})}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
			},
		},
		TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
		ProcessorManager:   &fake.ProcessorManager{},
		EventsPublisher:    &fake.Publisher{},
	}

	metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
	metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{
		uint8(peer.TxValidationCode_BAD_PROPOSAL_TXID),
		uint8(peer.TxValidationCode_VALID),
	}
	block := &common.BlockMetadata{Metadata: metadata}

	event, err := c.HandleEndorserTransaction(ctx, block, CommitTx{
		TxID: txID, BlkNum: 5, TxNum: 0, Raw: mustMarshalProto(t, forged), Envelope: forged,
	})
	require.NoError(t, err)
	require.Equal(t, fdriver.Invalid, event.ValidationCode)
	require.Empty(t, stored, "the envelope of an invalid transaction is not stored")

	event, err = c.HandleEndorserTransaction(ctx, block, CommitTx{
		TxID: txID, BlkNum: 5, TxNum: 1, Raw: mustMarshalProto(t, honest), Envelope: honest,
	})
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, event.ValidationCode, event.ValidationMessage)
	require.Equal(t, fdriver.Valid, status[txID])
	require.Equal(t, mustMarshalProto(t, honest), stored[txID])
}

// TestShadowedEnvelopes checks that an invalid envelope of any header type is
// shadowed exactly when a valid endorser transaction in the same block carries
// its txID.
func TestShadowedEnvelopes(t *testing.T) {
	t.Parallel()
	const (
		valid       = peer.TxValidationCode_VALID
		forged      = peer.TxValidationCode_BAD_PROPOSAL_TXID
		mvcc        = peer.TxValidationCode_MVCC_READ_CONFLICT
		dupTxID     = peer.TxValidationCode_DUPLICATE_TXID
		unsupported = peer.TxValidationCode_UNSUPPORTED_TX_PAYLOAD
	)
	endorser, config, message := common.HeaderType_ENDORSER_TRANSACTION, common.HeaderType_CONFIG, common.HeaderType_MESSAGE
	// tx is an envelope with txID id and header type typ, or raw when set.
	type tx struct {
		id  string
		typ common.HeaderType
		raw []byte
	}
	block := func(txs []tx, flags ...peer.TxValidationCode) *common.Block {
		data := make([][]byte, len(txs))
		for i, tx := range txs {
			data[i] = tx.raw
			if data[i] == nil {
				chdr := protoutil.MarshalOrPanic(&common.ChannelHeader{TxId: tx.id, Type: int32(tx.typ)})
				data[i] = protoutil.MarshalOrPanic(&common.Envelope{Payload: protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: chdr}})})
			}
		}
		filter := make([]byte, len(flags))
		for i, f := range flags {
			filter[i] = uint8(f)
		}
		return &common.Block{Data: &common.BlockData{Data: data}, Metadata: &common.BlockMetadata{Metadata: [][]byte{nil, nil, filter}}}
	}

	mixed := block([]tx{
		{id: "a", typ: endorser},         // forged, shadowed by 1
		{id: "a", typ: endorser},         // honest
		{id: "b", typ: endorser},         // invalid, no valid twin
		{id: "a", typ: endorser},         // duplicate after the honest one
		{id: "a", typ: config},           // invalid config with the honest txID
		{id: "a", typ: message},          // invalid message with the honest txID
		{id: "c", typ: endorser},         // valid
		{id: "d", typ: config},           // valid config: shadows nothing
		{id: "d", typ: endorser},         // invalid endorser, its txID only on a valid config
		{raw: []byte("not an envelope")}, // unparsable
	}, forged, valid, mvcc, dupTxID, forged, unsupported, valid, valid, mvcc, forged)
	assert.Equal(t, []bool{true, false, false, true, true, true, false, false, false, false}, ShadowedEnvelopes(mixed))

	two := []tx{{id: "a", typ: endorser}, {id: "a", typ: endorser}}
	assert.Nil(t, ShadowedEnvelopes(block(two, valid, valid)), "all valid")
	assert.Nil(t, ShadowedEnvelopes(block(two, forged, mvcc)), "no valid envelope")
	assert.Equal(t, []bool{false, true}, ShadowedEnvelopes(block(two, valid)), "an index beyond the flags counts as invalid")
	assert.Nil(t, ShadowedEnvelopes(&common.Block{Data: mixed.Data}), "no metadata")
	assert.Nil(t, ShadowedEnvelopes(&common.Block{Data: mixed.Data, Metadata: &common.BlockMetadata{}}), "no flags")
}

// TestCommitBlockSkipsShadowedEnvelope commits a block holding a forged and an
// honest envelope with one txID and checks that only the honest envelope
// reaches the handler, so the transaction gets a single finality event.
func TestCommitBlockSkipsShadowedEnvelope(t *testing.T) {
	t.Parallel()
	envelope := func(txID string) []byte {
		chdr := protoutil.MarshalOrPanic(&common.ChannelHeader{TxId: txID, Type: int32(common.HeaderType_ENDORSER_TRANSACTION)})
		payload := protoutil.MarshalOrPanic(&common.Payload{Header: &common.Header{ChannelHeader: chdr}})
		return protoutil.MarshalOrPanic(&common.Envelope{Payload: payload})
	}
	metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
	metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{
		uint8(peer.TxValidationCode_BAD_PROPOSAL_TXID),
		uint8(peer.TxValidationCode_VALID),
	}
	block := &common.Block{
		Header:   &common.BlockHeader{Number: 3},
		Data:     &common.BlockData{Data: [][]byte{envelope("tx1"), envelope("tx1")}},
		Metadata: &common.BlockMetadata{Metadata: metadata},
	}

	var handled []cdriver.TxNum
	c := &Committer{
		logger:             logger,
		ChannelConfig:      &fake.ChannelConfig{IDValue: "ch", CommitParallelismValue: 1},
		metrics:            NewMetrics(noop.NewTracerProvider(), &fake.MetricsProvider{}, "testNet", "testChannel"),
		events:             make(chan FinalityEvent, 2),
		configCaughtUp:     true,
		DependencyResolver: NewParallelDependencyResolver(),
		Handlers: map[common.HeaderType]TransactionHandler{
			common.HeaderType_ENDORSER_TRANSACTION: func(ctx context.Context, _ *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error) {
				handled = append(handled, tx.TxNum)
				return &FinalityEvent{Ctx: ctx, TxID: tx.TxID, ValidationCode: fdriver.Valid}, nil
			},
		},
	}
	require.NoError(t, c.commitBlockOnce(t.Context(), block))
	require.Equal(t, []cdriver.TxNum{1}, handled)
	require.Len(t, c.events, 1)
}
