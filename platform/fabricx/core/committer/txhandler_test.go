/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"reflect"
	"testing"
	"unsafe"

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

func setLogger(com *commoncommitter.Committer) *commoncommitter.Committer {
	logger := logging.MustGetLogger("test")
	field := reflect.ValueOf(com).Elem().FieldByName("logger")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(logger))
	return com
}

func TestRegisterTransactionHandler(t *testing.T) {
	t.Parallel()

	com := &commoncommitter.Committer{
		Handlers: make(map[cb.HeaderType]commoncommitter.TransactionHandler),
	}
	RegisterTransactionHandler(com)
	require.NotNil(t, com.Handlers[cb.HeaderType_MESSAGE])
}

func TestHandleFabricxTransactionShortTransactionsFilterReturnsError(t *testing.T) {
	t.Parallel()

	h := NewHandler(setLogger(&commoncommitter.Committer{}))

	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{
			{}, // SIGNATURES
			{}, // LAST_CONFIG
			{}, // TRANSACTIONS_FILTER -- empty
		},
	}
	tx := commoncommitter.CommitTx{TxNum: 0, TxID: "victim-tx"}

	require.NotPanics(t, func() {
		event, err := h.HandleFabricxTransaction(t.Context(), blkMetadata, tx)
		require.Error(t, err)
		require.Nil(t, event)
		require.Contains(t, err.Error(), "transaction filter has no entry")
	})
}

func TestHandleFabricxTransactionShortMetadataSliceReturnsError(t *testing.T) {
	t.Parallel()

	h := NewHandler(setLogger(&commoncommitter.Committer{}))

	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{
			{}, // SIGNATURES
			{}, // LAST_CONFIG
		},
	}
	tx := commoncommitter.CommitTx{TxNum: 0, TxID: "victim-tx"}

	require.NotPanics(t, func() {
		event, err := h.HandleFabricxTransaction(t.Context(), blkMetadata, tx)
		require.Error(t, err)
		require.Nil(t, event)
		require.Contains(t, err.Error(), "lacks transaction filter")
	})
}

func TestHandleFabricxTransactionCOMMITTEDSuccess(t *testing.T) {
	t.Parallel()

	fakeVault := &fake.Vault{
		StatusFn: func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
			return driver.Valid, "valid", nil
		},
	}

	com := setLogger(&commoncommitter.Committer{Vault: fakeVault})
	h := NewHandler(com)

	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{nil, nil, {uint8(committerpb.Status_COMMITTED)}},
	}
	tx := commoncommitter.CommitTx{TxNum: 0, TxID: "tx1"}

	event, err := h.HandleFabricxTransaction(t.Context(), blkMetadata, tx)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, driver.Valid, event.ValidationCode)
}

func TestHandleFabricxTransactionCOMMITTEDErrDiscardTX(t *testing.T) {
	t.Parallel()

	fakeVault := &fake.Vault{
		StatusFn: func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
			return driver.Unknown, "", errors.Wrapf(commoncommitter.ErrDiscardTX, "simulated discard")
		},
	}

	com := setLogger(&commoncommitter.Committer{Vault: fakeVault})
	h := NewHandler(com)

	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{nil, nil, {uint8(committerpb.Status_COMMITTED)}},
	}
	tx := commoncommitter.CommitTx{TxNum: 0, TxID: "tx1"}

	event, err := h.HandleFabricxTransaction(t.Context(), blkMetadata, tx)
	require.Error(t, err)
	require.Nil(t, event)
	require.ErrorContains(t, err, "discarding endorser transaction")
}

func TestHandleFabricxTransactionCOMMITTEDOtherError(t *testing.T) {
	t.Parallel()

	fakeVault := &fake.Vault{
		StatusFn: func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
			return driver.Unknown, "", errors.New("simulated error")
		},
	}

	com := setLogger(&commoncommitter.Committer{Vault: fakeVault})
	h := NewHandler(com)

	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{nil, nil, {uint8(committerpb.Status_COMMITTED)}},
	}
	tx := commoncommitter.CommitTx{TxNum: 0, TxID: "tx1"}

	event, err := h.HandleFabricxTransaction(t.Context(), blkMetadata, tx)
	require.Error(t, err)
	require.Nil(t, event)
	require.ErrorContains(t, err, "committing endorser transaction")
	require.ErrorContains(t, err, "simulated error")
}

func TestHandleFabricxTransactionDiscardSuccess(t *testing.T) {
	t.Parallel()

	fakeVault := &fake.Vault{
		StatusFn: func(context.Context, cdriver.TxID) (driver.ValidationCode, string, error) {
			return driver.Invalid, "invalid", nil
		},
	}

	com := setLogger(&commoncommitter.Committer{Vault: fakeVault})
	h := NewHandler(com)

	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{nil, nil, {uint8(committerpb.Status_ABORTED_SIGNATURE_INVALID)}},
	}
	tx := commoncommitter.CommitTx{TxNum: 0, TxID: "tx1"}

	event, err := h.HandleFabricxTransaction(t.Context(), blkMetadata, tx)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, driver.Invalid, event.ValidationCode)
}

func TestConvertValidationCode(t *testing.T) {
	t.Parallel()

	require.Equal(t, driver.Valid, convertValidationCode(committerpb.Status_COMMITTED))
	require.Equal(t, driver.Invalid, convertValidationCode(committerpb.Status_ABORTED_SIGNATURE_INVALID))
	require.Equal(t, driver.Invalid, convertValidationCode(committerpb.Status_STATUS_UNSPECIFIED))
}
