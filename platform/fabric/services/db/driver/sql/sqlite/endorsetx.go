/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

type EndorseTxStore struct {
	*common.EndorseTxStore
}

func NewEndorseTxStore(dbs *drivercommon.RWDB, tables common.TableNames) (*EndorseTxStore, error) {
	return buildEndorseTxStore(dbs.ReadDB, sqlite.NewRetryWriteDB(dbs.WriteDB), tables.EndorseTx), nil
}

func buildEndorseTxStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, table string) *EndorseTxStore {
	return &EndorseTxStore{EndorseTxStore: common.NewEndorseTxStore(writeDB, readDB, table, &sqlite.ErrorMapper{})}
}
