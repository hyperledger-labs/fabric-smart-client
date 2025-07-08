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

type AuditInfoStore struct {
	*common3.AuditInfoStore
}

func NewAuditInfoStore(dbs *common2.RWDB, tables common3.TableNames) (*AuditInfoStore, error) {
	return newAuditInfoStore(dbs.ReadDB, dbs.WriteDB, tables.AuditInfo), nil
}

func newAuditInfoStore(readDB, writeDB *sql.DB, table string) *AuditInfoStore {
	return &AuditInfoStore{AuditInfoStore: common3.NewAuditInfoStore(readDB, writeDB, table, &ErrorMapper{}, NewConditionInterpreter())}
}
