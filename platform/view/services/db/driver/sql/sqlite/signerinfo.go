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

type SignerInfoStore struct {
	*common.SignerInfoStore
}

func NewSignerInfoStore(dbs *common3.RWDB, tables common.TableNames) (*SignerInfoStore, error) {
	return newSignerInfoStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.SignerInfo), nil
}

func newSignerInfoStore(readDB *sql.DB, writeDB common.WriteDB, table string) *SignerInfoStore {
	return &SignerInfoStore{SignerInfoStore: common.NewSignerInfoStore(writeDB, readDB, table, &errorMapper{}, NewConditionInterpreter())}
}
