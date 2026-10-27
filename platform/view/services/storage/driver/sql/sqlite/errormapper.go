/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	stderrors "errors"

	"modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// errorMap maps SQLite result codes onto driver sentinels. modernc enables
// extended result codes on every connection, so an error carries the extended
// code; sentinelFor looks it up first and falls back to the primary code (the
// low byte). A primary code is listed only when every extended code under it means
// the same sentinel: 5 covers SQLITE_BUSY_RECOVERY (261), SQLITE_BUSY_SNAPSHOT
// (517) and SQLITE_BUSY_TIMEOUT (773), while the primary SQLITE_CONSTRAINT (19)
// also covers foreign key, NOT NULL and CHECK failures, so only the unique and
// primary key codes are listed.
var errorMap = map[int]error{
	1555: driver.UniqueKeyViolation, // SQLITE_CONSTRAINT_PRIMARYKEY
	2067: driver.UniqueKeyViolation, // SQLITE_CONSTRAINT_UNIQUE
	5:    driver.SqlBusy,            // SQLITE_BUSY
}

// ErrorMapper maps modernc SQLite errors onto the sentinels of the storage driver
// package, so that callers match them with errors.Is independently of the backend.
type ErrorMapper struct{}

// WrapError returns err wrapped around the sentinel its result code maps to, or err
// unchanged when it is not a SQLite error or its code is not mapped.
func (*ErrorMapper) WrapError(err error) error {
	var sqliteErr *sqlite.Error
	if !stderrors.As(err, &sqliteErr) {
		return err
	}
	mappedErr := sentinelFor(sqliteErr.Code())
	if mappedErr == nil {
		logger.Warnf("unmapped sqlite error with code [%d]", sqliteErr.Code())
		return sqliteErr
	}
	return errors.Wrapf(mappedErr, "%s", err)
}

// sentinelFor returns the sentinel that code maps to, or nil when none does.
func sentinelFor(code int) error {
	if mappedErr, ok := errorMap[code]; ok {
		return mappedErr
	}
	return errorMap[code&0xff]
}
