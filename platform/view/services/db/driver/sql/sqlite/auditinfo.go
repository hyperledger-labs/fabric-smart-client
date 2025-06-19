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

type AuditInfoStore struct {
	*common.AuditInfoStore
}

func NewAuditInfoStore(dbs *common3.RWDB, tables common.TableNames) (*AuditInfoStore, error) {
	return newAuditInfoStore(dbs.ReadDB, dbs.WriteDB, tables.AuditInfo), nil
}

func newAuditInfoStore(readDB, writeDB *sql.DB, table string) *AuditInfoStore {
	return &AuditInfoStore{AuditInfoStore: common.NewAuditInfoStore(readDB, writeDB, table, &ErrorMapper{}, NewConditionInterpreter())}
}
