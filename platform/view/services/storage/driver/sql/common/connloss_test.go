/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	storagedriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The committer retries a block only on a failure the store's error wrapper has
// mapped, so a write must return the mapped error rather than the driver's own.
func TestPut_MapsConnectionLoss(t *testing.T) {
	t.Parallel()
	mapper := &postgres.ErrorMapper{}
	for name, tc := range map[string]struct {
		// expect registers the statements the write runs before its INSERT.
		expect func(sqlmock.Sqlmock)
		put    func(ctx context.Context, db *sql.DB) error
	}{
		"PutData": {
			put: func(ctx context.Context, db *sql.DB) error {
				return common.NewSimpleKeyDataStore(db, db, "t", mapper).PutData(ctx, "tx1", []byte("env"))
			},
		},
		"PutSigner": {
			put: func(ctx context.Context, db *sql.DB) error {
				return common.NewSignerInfoStore(db, db, "t", mapper).PutSigner(ctx, view.Identity("signer"))
			},
		},
		"PutAuditInfo": {
			put: func(ctx context.Context, db *sql.DB) error {
				return common.NewAuditInfoStore(db, db, "t", mapper).PutAuditInfo(ctx, view.Identity("id"), []byte("info"))
			},
		},
		"PutBindings": {
			// The long-term identity is resolved first; it is not bound yet.
			expect: func(m sqlmock.Sqlmock) {
				m.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"long_term_id"}))
			},
			put: func(ctx context.Context, db *sql.DB) error {
				return common.NewBindingStore(db, db, "t", mapper).PutBindings(ctx, view.Identity("lt"), view.Identity("eph"))
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, mockDB, err := sqlmock.New()
			require.NoError(t, err)
			if tc.expect != nil {
				tc.expect(mockDB)
			}
			mockDB.ExpectExec("INSERT INTO t").WillReturnError(io.ErrUnexpectedEOF)

			require.ErrorIs(t, tc.put(t.Context(), db), storagedriver.ErrConnectionLost)
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}
