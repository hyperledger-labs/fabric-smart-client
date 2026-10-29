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

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type dummyUniqueErrorWrapper struct{}

func (*dummyUniqueErrorWrapper) WrapError(_ error) error {
	return driver.UniqueKeyViolation
}

// passthroughErrorWrapper returns a wrapper that maps no error, as a real wrapper does
// for an error it does not recognize.
func passthroughErrorWrapper() *mock.SQLErrorWrapper {
	w := &mock.SQLErrorWrapper{}
	w.WrapErrorStub = func(err error) error { return err }
	return w
}

func TestFilterExistingSigners(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	id1 := view.Identity("signer1")
	id2 := view.Identity("signer2")

	query := "SELECT id FROM signer_info WHERE id IN ($1,$2)"

	mockDB.
		ExpectQuery(query).
		WithArgs(id1.UniqueID(), id2.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"id"}).
			AddRow(id1.UniqueID()).
			AddRow(id2.UniqueID()))

	store := mockSignerInfoStore(db, false)
	result, err := store.FilterExistingSigners(context.Background(), id1, id2)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
	require.Subset(t, result, []view.Identity{id1, id2})
}

func TestFilterExistingSigners_QueryError(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	id := view.Identity("signer1")
	query := "SELECT id FROM signer_info WHERE id IN ($1)"

	mockDB.
		ExpectQuery(query).
		WithArgs(id.UniqueID()).
		WillReturnError(sql.ErrConnDone)

	store := mockSignerInfoStore(db, false)
	_, err = store.FilterExistingSigners(context.Background(), id)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.Error(t, err)
	require.Contains(t, err.Error(), "error querying db")
}

func TestFilterExistingSigners_ScanError(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	id := view.Identity("signer1")
	query := "SELECT id FROM signer_info WHERE id IN ($1)"

	mockDB.
		ExpectQuery(query).
		WithArgs(id.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"id"}).AddRow(nil)) // nil triggers scan error

	store := mockSignerInfoStore(db, false)
	_, err = store.FilterExistingSigners(context.Background(), id)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed scanning row")
}

func TestPutSigner(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	id := view.Identity("signer1")

	query := "INSERT INTO signer_info (id) VALUES ($1)"

	mockDB.
		ExpectExec(query).
		WithArgs(id.UniqueID()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	store := mockSignerInfoStore(db, false)
	err = store.PutSigner(context.Background(), id)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
}

func TestPutSigner_ExecError(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	id := view.Identity("signer1")
	query := "INSERT INTO signer_info (id) VALUES ($1)"

	mockDB.
		ExpectExec(query).
		WithArgs(id.UniqueID()).
		WillReturnError(sql.ErrConnDone)

	store := mockSignerInfoStore(db, false)
	err = store.PutSigner(context.Background(), id)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed executing query")
}

func TestPutSigner_UniqueViolation(t *testing.T) {
	t.Parallel()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	id := view.Identity("signer1")
	query := "INSERT INTO signer_info (id) VALUES ($1)"

	mockDB.
		ExpectExec(query).
		WithArgs(id.UniqueID()).
		WillReturnError(sql.ErrNoRows)

	store := mockSignerInfoStore(db, true)
	err = store.PutSigner(context.Background(), id)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
}

func mockSignerInfoStore(db *sql.DB, isdummyUniqueErrorWrapper bool) *common.SignerInfoStore {
	if isdummyUniqueErrorWrapper {
		return common.NewSignerInfoStore(db, db, "signer_info", &dummyUniqueErrorWrapper{})
	}
	return common.NewSignerInfoStore(db, db, "signer_info", passthroughErrorWrapper())
}
