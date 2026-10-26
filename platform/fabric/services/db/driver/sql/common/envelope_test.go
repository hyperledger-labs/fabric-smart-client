/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
)

func TestEndorseTX_GetData(t *testing.T) { //nolint:paralleltest
	GetData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEndorseTXStore(db).GetEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_GetData_NoData(t *testing.T) { //nolint:paralleltest
	GetDataNoData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEndorseTXStore(db).GetEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_ExistData_True(t *testing.T) { //nolint:paralleltest
	ExistDataTrue(t, func(db *sql.DB, key string) (bool, error) {
		return mockEndorseTXStore(db).ExistsEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_ExistData_False(t *testing.T) { //nolint:paralleltest
	ExistDataFalse(t, func(db *sql.DB, key string) (bool, error) {
		return mockEndorseTXStore(db).ExistsEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_PutData_Success(t *testing.T) { //nolint:paralleltest
	PutDataSuccess(t, func(db *sql.DB, key string, data []byte) error {
		return mockEndorseTXStore(db).PutEndorseTx(context.Background(), key, data)
	})
}

func TestEndorseTX_PutData_Conflict(t *testing.T) { //nolint:paralleltest
	PutDataConflict(t, func(db *sql.DB, key string, data []byte) error {
		return mockEndorseTXStore(db).PutEndorseTx(context.Background(), key, data)
	})
}

func mockEndorseTXStore(db *sql.DB) *common.EndorseTxStore {
	return common.NewEndorseTxStore(db, db, "test_table", &mock.SQLErrorWrapper{})
}

// TestEnvelope_ExistData_WrapsError checks that a read failure passes through
// the store's error wrapper, which is what maps a lost connection onto
// driver.ErrConnectionLost for the committer to retry.
func TestEnvelope_ExistData_WrapsError(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mockDB.ExpectQuery("SELECT data").WillReturnError(errors.New("connection reset by peer"))

	wrapper := &mock.SQLErrorWrapper{}
	wrapper.WrapErrorStub = func(err error) error { return errors.Wrap(dbdriver.ErrConnectionLost, err.Error()) }

	_, err = common.NewEnvelopeStore(db, db, "test_table", wrapper).ExistsEnvelope(t.Context(), "k")
	require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
}
