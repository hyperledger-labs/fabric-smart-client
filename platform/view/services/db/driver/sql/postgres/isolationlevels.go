/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
)

var isolationLevels = isolationLevelMapper{
	driver.LevelDefault:         sql.LevelDefault,
	driver.LevelReadUncommitted: sql.LevelReadUncommitted,
	driver.LevelReadCommitted:   sql.LevelReadCommitted,
	driver.LevelWriteCommitted:  sql.LevelWriteCommitted,
	driver.LevelRepeatableRead:  sql.LevelRepeatableRead,
	driver.LevelSnapshot:        sql.LevelSnapshot,
	driver.LevelSerializable:    sql.LevelSerializable,
	driver.LevelLinearizable:    sql.LevelLinearizable,
}

type isolationLevelMapper map[driver.IsolationLevel]sql.IsolationLevel

func (m isolationLevelMapper) Map(level driver.IsolationLevel) (sql.IsolationLevel, error) {
	il, ok := m[level]
	if !ok {
		return 0, errors.Errorf("isolation level [%d] not defined for postgres", il)
	}
	return il, nil
}
