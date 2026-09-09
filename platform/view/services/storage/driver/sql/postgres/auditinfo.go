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
	return buildAuditInfoStore(dbs.ReadDB, dbs.WriteDB, tables.AuditInfo), nil
}

func buildAuditInfoStore(readDB, writeDB *sql.DB, table string) *AuditInfoStore {
	return &AuditInfoStore{AuditInfoStore: common3.NewAuditInfoStore(writeDB, readDB, table, &ErrorMapper{})}
}
