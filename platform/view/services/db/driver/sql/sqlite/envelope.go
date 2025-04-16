/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type EnvelopePersistence struct {
	*common.EnvelopePersistence
}

func NewEnvelopePersistence(opts Opts) (*EnvelopePersistence, error) {
	readDB, writeDB, err := openRWDBs(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newEnvelopePersistence(readDB, NewRetryWriteDB(writeDB), tables.Envelope), nil
}

func newEnvelopePersistence(readDB *sql.DB, writeDB common.WriteDB, table string) *EnvelopePersistence {
	return &EnvelopePersistence{EnvelopePersistence: common.NewEnvelopePersistence(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
