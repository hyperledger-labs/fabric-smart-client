/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	cvault "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fvault "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// A responder on this node may hold a transaction's read-write set open when
// the committer reaches the transaction's block. The block is retried, and once
// the responder closes its set the transaction gets Fabric's verdict, with the
// writes of the block envelope.
func TestCommitRetriesWhileResponderHoldsRWSetOpen(t *testing.T) {
	t.Parallel()

	const (
		beforeBlock  = "before the block"
		afterStatus  = "after the status read"
		beforeCommit = "between loading the envelope and committing"
	)
	for _, tc := range []struct {
		opens string
		code  peer.TxValidationCode
		want  fdriver.ValidationCode
	}{
		{opens: beforeBlock, code: peer.TxValidationCode_VALID, want: fdriver.Valid},
		{opens: afterStatus, code: peer.TxValidationCode_VALID, want: fdriver.Valid},
		{opens: beforeCommit, code: peer.TxValidationCode_VALID, want: fdriver.Valid},
		{opens: beforeBlock, code: peer.TxValidationCode_MVCC_READ_CONFLICT, want: fdriver.Invalid},
	} {
		t.Run(tc.opens+"/"+tc.code.String(), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			store, err := fvault.OpenSqliteVault("node", t.TempDir())
			require.NoError(t, err)
			defer func() { _ = store.Close() }()
			v := vault.NewVault(fvault.NewCachedVault(store, 0), &disabled.Provider{}, noop.NewTracerProvider())

			// The responder writes a value of its own, so the test can tell
			// whose writes were committed.
			var responder fdriver.RWSet
			openResponder := func(at string) {
				if tc.opens != at || responder != nil {
					return
				}
				var err error
				responder, err = v.NewRWSet(ctx, "tx1")
				require.NoError(t, err)
				require.NoError(t, responder.SetState("ns", "key", []byte("responder")))
			}

			c := &Committer{
				logger:            logger,
				metrics:           NewMetrics(noop.NewTracerProvider(), &fake.MetricsProvider{}, "net", "ch"),
				ChannelConfig:     &fake.ChannelConfig{IDValue: "ch", Retries: 1},
				Vault:             v,
				ProcessNamespaces: []string{"ns"},
				EnvelopeService: &fake.EnvelopeService{
					StoreEnvelopeFn: func(context.Context, string, any) error {
						openResponder(afterStatus)
						return nil
					},
				},
				TransactionManager: &fake.TransactionManager{
					NewProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
						return &fake.ProcessedTransaction{}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
					},
				},
				ProcessorManager: &fake.ProcessorManager{
					ProcessByIDFn: func(context.Context, string, cdriver.TxID) error {
						openResponder(beforeCommit)
						return nil
					},
				},
				RWSetLoaderService: &fake.RWSetLoader{
					InspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
						return &fake.RWSet{NamespacesList: []cdriver.Namespace{"ns"}}, nil, nil
					},
					// Stands in for decoding the block envelope into the vault.
					GetFromEnvelopeFn: func(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
						rws, err := v.NewRWSet(ctx, txID)
						if err != nil {
							return nil, nil, err
						}
						return rws, nil, rws.SetState("ns", "key", []byte("ordered"))
					},
				},
			}
			openResponder(beforeBlock)

			metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
			metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(tc.code)}
			attempts := 0
			var firstErr error
			c.commitBlock = func(ctx context.Context, _ *common.Block) error {
				attempts++
				_, err := c.HandleEndorserTransaction(ctx, &common.BlockMetadata{Metadata: metadata}, CommitTx{
					TxID: "tx1", BlkNum: 7, Envelope: &common.Envelope{},
				})
				if attempts == 1 {
					firstErr = err
					// The responder closes its set while the block waits.
					responder.Done()
				}
				return err
			}

			require.NoError(t, c.Commit(ctx, &common.Block{Header: &common.BlockHeader{Number: 7}}))
			require.Equal(t, 2, attempts)
			require.ErrorIs(t, firstErr, cvault.ErrRWSetOpen)

			vc, _, err := c.Status(ctx, "tx1")
			require.NoError(t, err)
			require.Equal(t, tc.want, vc)
			require.False(t, v.RWSExists(ctx, "tx1"))
			if tc.want == fdriver.Valid {
				qe, err := v.NewQueryExecutor(ctx)
				require.NoError(t, err)
				defer func() { require.NoError(t, qe.Done()) }()
				read, err := qe.GetState(ctx, "ns", "key")
				require.NoError(t, err)
				require.Equal(t, []byte("ordered"), read.Raw)
			}
		})
	}
}
