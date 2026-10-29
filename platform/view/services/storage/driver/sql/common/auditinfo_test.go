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

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestGetAuditInfo(t *testing.T) {
	t.Parallel()
	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	input, output := view.Identity("an_id"), []byte("some_result")
	mockDB.
		ExpectQuery("SELECT audit_info FROM audit_info WHERE id = $1").
		WithArgs(input.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"audit_info"}).AddRow(output))

	info, err := mockAuditInfoStore(db).GetAuditInfo(context.Background(), input)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
	require.Equal(t, output, info)
}

func TestPutAuditInfo(t *testing.T) {
	t.Parallel()
	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	input, output := view.Identity("an_id"), []byte("some_result")
	mockDB.
		ExpectExec("INSERT INTO audit_info (id,audit_info) VALUES ($1,$2)").
		WithArgs(input.UniqueID(), output).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = mockAuditInfoStore(db).PutAuditInfo(context.Background(), input, output)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
}

func mockAuditInfoStore(db *sql.DB) *common.AuditInfoStore {
	return common.NewAuditInfoStore(db, db, "audit_info", passthroughErrorWrapper())
}
