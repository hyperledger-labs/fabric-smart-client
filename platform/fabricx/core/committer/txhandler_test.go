/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"reflect"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	commoncommitter "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// newTestCommitter builds a Committer usable from outside its own package: the
// commit/discard paths dereference the unexported logger field, which no
// exported constructor lets us set without wiring all of New's collaborators.
func newTestCommitter(t *testing.T, vault driver.Vault) *commoncommitter.Committer {
	t.Helper()

	com := &commoncommitter.Committer{Vault: vault}
	f := reflect.ValueOf(com).Elem().FieldByName("logger")
	require.True(t, f.IsValid(), "Committer.logger was renamed, update this helper")
	reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem().Set(reflect.ValueOf(logging.MustGetLogger()))
	return com
}

func statusFn(vc driver.ValidationCode, msg string, err error) func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
	return func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
		return vc, msg, err
	}
}

// txFilter returns block metadata carrying statuses as the TRANSACTIONS_FILTER entry.
func txFilter(statuses ...committerpb.Status) [][]byte {
	filter := make([]byte, len(statuses))
	for i, s := range statuses {
		filter[i] = byte(s)
	}
	return [][]byte{nil, nil, filter}
}

func TestRegisterTransactionHandler(t *testing.T) {
	t.Parallel()

	com := &commoncommitter.Committer{Handlers: map[cb.HeaderType]commoncommitter.TransactionHandler{}}
	RegisterTransactionHandler(com)
	require.NotNil(t, com.Handlers[cb.HeaderType_MESSAGE])
}

func TestHandleFabricxTransaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata [][]byte
		statusFn func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error)
		wantCode driver.ValidationCode
		wantErr  []string
	}{
		// Regression test: block metadata sized exactly at the
		// TRANSACTIONS_FILTER index used to pass the `len(...) < statusIdx`
		// guard and then panic on Metadata[statusIdx] (index out of range).
		// A malicious or buggy committer/notifier can deliver such a block,
		// and nothing in the delivery -> committer call chain recovers panics,
		// so this crashed the FSC client process.
		{
			name:     "metadata exactly at transactions filter index is rejected, not a panic",
			metadata: [][]byte{nil, nil}, // SIGNATURES, LAST_CONFIG; no TRANSACTIONS_FILTER
			wantErr:  []string{"lacks transaction filter"},
		},
		// Regression test for the same DoS, one level in: the
		// TRANSACTIONS_FILTER entry is present but reports fewer statuses
		// than the block has transactions (here: none at all), so indexing
		// it with tx.TxNum panicked.
		{
			name:     "transaction filter shorter than the reported tx index is rejected, not a panic",
			metadata: txFilter(), // present but empty
			wantErr:  []string{"transaction filter has no entry"},
		},
		{
			name:     "committed tx already valid in the vault is not committed twice",
			metadata: txFilter(committerpb.Status_COMMITTED),
			statusFn: statusFn(driver.Valid, "valid", nil),
			wantCode: driver.Valid,
		},
		{
			name:     "committed tx propagates a vault failure",
			metadata: txFilter(committerpb.Status_COMMITTED),
			statusFn: statusFn(driver.Unknown, "", errors.New("simulated error")),
			wantErr:  []string{"committing endorser transaction", "simulated error"},
		},
		{
			name:     "aborted tx already invalid in the vault is not discarded twice",
			metadata: txFilter(committerpb.Status_ABORTED_SIGNATURE_INVALID),
			statusFn: statusFn(driver.Invalid, "invalid", nil),
			wantCode: driver.Invalid,
		},
		{
			name:     "aborted tx propagates a discard failure",
			metadata: txFilter(committerpb.Status_ABORTED_SIGNATURE_INVALID),
			statusFn: statusFn(driver.Unknown, "", errors.New("simulated error")),
			wantErr:  []string{"discarding endorser transaction", "simulated error"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(newTestCommitter(t, &fake.Vault{StatusFn: test.statusFn}))
			event, err := h.HandleFabricxTransaction(t.Context(), &cb.BlockMetadata{Metadata: test.metadata}, commoncommitter.CommitTx{TxID: "tx1"})

			if len(test.wantErr) > 0 {
				require.Nil(t, event)
				for _, want := range test.wantErr {
					require.ErrorContains(t, err, want)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantCode, event.ValidationCode)
		})
	}
}

// A committed tx that the vault refuses with ErrDiscardTX must come back as an
// invalid event carrying the vault's reason, not as a handler error: callers
// waiting on finality need the verdict, and the tx must still be discarded.
func TestHandleFabricxTransactionErrDiscardTXDiscardsTheTransaction(t *testing.T) {
	t.Parallel()

	// The commit path asks the vault for the status first and gets
	// ErrDiscardTX; the discard path that follows sees it already invalid.
	calls := 0
	vault := &fake.Vault{StatusFn: func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
		calls++
		if calls == 1 {
			return driver.Unknown, "", errors.Wrapf(commoncommitter.ErrDiscardTX, "simulated discard")
		}
		return driver.Invalid, "invalid", nil
	}}

	h := NewHandler(newTestCommitter(t, vault))
	event, err := h.HandleFabricxTransaction(t.Context(), &cb.BlockMetadata{Metadata: txFilter(committerpb.Status_COMMITTED)}, commoncommitter.CommitTx{TxID: "tx1"})

	require.NoError(t, err)
	require.Equal(t, driver.Invalid, event.ValidationCode)
	require.Contains(t, event.ValidationMessage, "simulated discard")
	require.Equal(t, 2, calls, "the transaction must also be discarded")
}

func TestConvertValidationCode(t *testing.T) {
	t.Parallel()

	require.Equal(t, driver.Valid, convertValidationCode(committerpb.Status_COMMITTED))
	require.Equal(t, driver.Invalid, convertValidationCode(committerpb.Status_STATUS_UNSPECIFIED))
}
