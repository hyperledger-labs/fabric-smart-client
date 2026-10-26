/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	storagedriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// The committer retries a block only on a failure the store's error wrapper has
// mapped, so a write must return the mapped error rather than the driver's own.
func TestPutData_MapsConnectionLoss(t *testing.T) {
	t.Parallel()
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	mockDB.ExpectExec("INSERT INTO envelopes").WillReturnError(io.ErrUnexpectedEOF)

	store := common2.NewSimpleKeyDataStore(db, db, "envelopes", &postgres.ErrorMapper{})
	err = store.PutData(t.Context(), "tx1", []byte("env"))

	require.ErrorIs(t, err, storagedriver.ErrConnectionLost)
	require.NoError(t, mockDB.ExpectationsWereMet())
}
