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

	input, info := view.Identity("an_id"), []byte("some_result")
	insert := "INSERT INTO audit_info (id,audit_info) VALUES ($1,$2) ON CONFLICT DO NOTHING"
	read := "SELECT audit_info FROM audit_info WHERE id = $1"

	for name, tc := range map[string]struct {
		inserted int64
		stored   []byte // returned by the read-back when the row was not inserted
		wantErr  error
	}{
		"new":             {inserted: 1},
		"same value":      {stored: info},
		"different value": {stored: []byte("other"), wantErr: driver.ErrConflict},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)

			mockDB.ExpectExec(insert).WithArgs(input.UniqueID(), info).WillReturnResult(sqlmock.NewResult(0, tc.inserted))
			if tc.inserted == 0 {
				mockDB.ExpectQuery(read).WithArgs(input.UniqueID()).WillReturnRows(mockDB.NewRows([]string{"audit_info"}).AddRow(tc.stored))
			}

			err = mockAuditInfoStore(db).PutAuditInfo(context.Background(), input, info)

			require.NoError(t, mockDB.ExpectationsWereMet())
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func mockAuditInfoStore(db *sql.DB) *common.AuditInfoStore {
	return common.NewAuditInfoStore(db, db, "audit_info", passthroughErrorWrapper(), 0)
}
