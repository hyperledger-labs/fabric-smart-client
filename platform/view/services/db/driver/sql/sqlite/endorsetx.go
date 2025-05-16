/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type EndorseTxStore struct {
	*common.EndorseTxStore
}

func NewEndorseTxStore(dbs *common3.RWDB, tables common.TableNames) (*EndorseTxStore, error) {
	return newEndorseTxStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.EndorseTx), nil
}

func newEndorseTxStore(readDB *sql.DB, writeDB common.WriteDB, table string) *EndorseTxStore {
	return &EndorseTxStore{EndorseTxStore: common.NewEndorseTxStore(writeDB, readDB, table, &errorMapper{}, NewConditionInterpreter())}
}
