/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestBuildStoresUseReadAndWriteDBs pins which pool each store talks to. The
// builders here take (readDB, writeDB) while the shared constructors take
// (writeDB, readDB), and both are *sql.DB on the Postgres side, so handing them
// straight through compiles while crossing the two. Each store gets two
// distinct mocks, so a read on the write pool or a write on the read pool is an
// unexpected call and fails.
func TestBuildStoresUseReadAndWriteDBs(t *testing.T) {
	t.Parallel()

	tables, err := common3.GetTableNames("test")
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		readSQL  string
		writeSQL string
		read     func(*common2.RWDB) error
		write    func(*common2.RWDB) error
	}{
		"KeyValue": {
			"SELECT val FROM " + tables.KVS,
			"INSERT INTO " + tables.KVS,
			func(dbs *common2.RWDB) error {
				s, err := NewKeyValueStore(dbs, tables)
				if err != nil {
					return err
				}
				_, err = s.GetState(context.Background(), "ns", "k")
				return err
			},
			func(dbs *common2.RWDB) error {
				s, err := NewKeyValueStore(dbs, tables)
				if err != nil {
					return err
				}
				return s.SetState(context.Background(), "ns", "k", []byte("v"))
			},
		},
		"SignerInfo": {
			"SELECT id FROM " + tables.SignerInfo,
			"INSERT INTO " + tables.SignerInfo,
			func(dbs *common2.RWDB) error {
				s, err := NewSignerInfoStore(dbs, tables)
				if err != nil {
					return err
				}
				_, err = s.FilterExistingSigners(context.Background(), view.Identity("id"))
				return err
			},
			func(dbs *common2.RWDB) error {
				s, err := NewSignerInfoStore(dbs, tables)
				if err != nil {
					return err
				}
				return s.PutSigner(context.Background(), view.Identity("id"))
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			readDB, readMock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = readDB.Close() })

			writeDB, writeMock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = writeDB.Close() })

			dbs := &common2.RWDB{ReadDB: readDB, WriteDB: writeDB}

			readMock.ExpectQuery(tc.readSQL).
				WillReturnRows(sqlmock.NewRows([]string{"val"}))
			require.NoError(t, tc.read(dbs))
			require.NoError(t, readMock.ExpectationsWereMet())
			require.NoError(t, writeMock.ExpectationsWereMet())

			writeMock.ExpectExec(tc.writeSQL).
				WillReturnResult(sqlmock.NewResult(1, 1))
			require.NoError(t, tc.write(dbs))
			require.NoError(t, writeMock.ExpectationsWereMet())
			require.NoError(t, readMock.ExpectationsWereMet())
		})
	}
}
