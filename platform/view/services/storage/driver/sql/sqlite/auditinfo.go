/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type AuditInfoStore struct {
	*sqlcommon.AuditInfoStore
}

func NewAuditInfoStore(dbs *drivercommon.RWDB, tables sqlcommon.TableNames) (*AuditInfoStore, error) {
	return buildAuditInfoStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.AuditInfo), nil
}

func buildAuditInfoStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, table string) *AuditInfoStore {
	return &AuditInfoStore{AuditInfoStore: sqlcommon.NewAuditInfoStore(writeDB, readDB, table, &ErrorMapper{})}
}
