/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type SignerInfoStore struct {
	*common.SignerInfoStore
}

func NewSignerInfoStore(dbs *common2.RWDB, tables common.TableNames) (*SignerInfoStore, error) {
	return newSignerInfoStore(dbs.ReadDB, dbs.WriteDB, tables.SignerInfo), nil
}

func newSignerInfoStore(readDB, writeDB *sql.DB, table string) *SignerInfoStore {
	return &SignerInfoStore{SignerInfoStore: common.NewSignerInfoStore(readDB, writeDB, table, &ErrorMapper{}, NewConditionInterpreter())}
}
