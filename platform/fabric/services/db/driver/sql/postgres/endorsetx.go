/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

type EndorseTxStore struct {
	*common.EndorseTxStore
}

func NewEndorseTxStore(dbs *common2.RWDB, tables common.TableNames) (*EndorseTxStore, error) {
	return newEndorseTxStore(dbs.ReadDB, dbs.WriteDB, tables.EndorseTx), nil
}

func newEndorseTxStore(readDB, writeDB *sql.DB, table string) *EndorseTxStore {
	return &EndorseTxStore{EndorseTxStore: common.NewEndorseTxStore(readDB, writeDB, table, &postgres2.ErrorMapper{}, postgres2.NewConditionInterpreter())}
}
