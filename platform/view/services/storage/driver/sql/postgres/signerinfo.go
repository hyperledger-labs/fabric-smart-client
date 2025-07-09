/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type SignerInfoStore struct {
	*common3.SignerInfoStore
}

func NewSignerInfoStore(dbs *common2.RWDB, tables common3.TableNames) (*SignerInfoStore, error) {
	return newSignerInfoStore(dbs.ReadDB, dbs.WriteDB, tables.SignerInfo), nil
}

func newSignerInfoStore(readDB, writeDB *sql.DB, table string) *SignerInfoStore {
	return &SignerInfoStore{SignerInfoStore: common3.NewSignerInfoStore(readDB, writeDB, table, &ErrorMapper{}, NewConditionInterpreter())}
}
