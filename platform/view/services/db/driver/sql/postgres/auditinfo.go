/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type AuditInfoPersistence struct {
	*common.AuditInfoPersistence
}

func NewAuditInfoPersistence(opts common.Opts, table string) (*AuditInfoPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newAuditInfoPersistence(readWriteDB, readWriteDB, table), nil
}

func newAuditInfoPersistence(readDB, writeDB *sql.DB, table string) *AuditInfoPersistence {
	return &AuditInfoPersistence{AuditInfoPersistence: common.NewAuditInfoPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
