/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	common4 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

type EndorseTxStore struct {
	*common.EndorseTxStore
}

func NewEndorseTxStore(dbs *common3.RWDB, tables common.TableNames) (*EndorseTxStore, error) {
	return newEndorseTxStore(dbs.ReadDB, sqlite.NewRetryWriteDB(dbs.WriteDB), tables.EndorseTx), nil
}

func newEndorseTxStore(readDB *sql.DB, writeDB common4.WriteDB, table string) *EndorseTxStore {
	return &EndorseTxStore{EndorseTxStore: common.NewEndorseTxStore(writeDB, readDB, table, &sqlite.ErrorMapper{}, sqlite.NewConditionInterpreter())}
}
