/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// IsolationLevels maps the isolation levels the pgx driver can begin a transaction
// with. Postgres has no WRITE COMMITTED or LINEARIZABLE level and pgx rejects both,
// so Map rejects LevelWriteCommitted and LevelLinearizable before a transaction is
// opened. pgx runs LevelSnapshot as REPEATABLE READ.
var IsolationLevels = isolationLevelMapper{
	driver.LevelDefault:         sql.LevelDefault,
	driver.LevelReadUncommitted: sql.LevelReadUncommitted,
	driver.LevelReadCommitted:   sql.LevelReadCommitted,
	driver.LevelRepeatableRead:  sql.LevelRepeatableRead,
	driver.LevelSnapshot:        sql.LevelSnapshot,
	driver.LevelSerializable:    sql.LevelSerializable,
}

type isolationLevelMapper map[driver.IsolationLevel]sql.IsolationLevel

func (m isolationLevelMapper) Map(level driver.IsolationLevel) (sql.IsolationLevel, error) {
	il, ok := m[level]
	if !ok {
		return 0, errors.Errorf("isolation level [%d] not defined for postgres", level)
	}
	return il, nil
}
