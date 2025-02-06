/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
)

var isolationLevels = isolationLevelMapper{
	driver.LevelDefault:      sql.LevelDefault,
	driver.LevelSerializable: sql.LevelSerializable,
}

type isolationLevelMapper map[driver.IsolationLevel]sql.IsolationLevel

func (m isolationLevelMapper) Map(level driver.IsolationLevel) (sql.IsolationLevel, error) {
	il, ok := m[level]
	if !ok {
		return 0, errors.Errorf("isolation level [%d] not defined for sqlite", il)
	}
	return il, nil
}
