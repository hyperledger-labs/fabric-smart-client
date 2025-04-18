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

type EnvelopePersistence struct {
	*common.EnvelopePersistence
}

func NewEnvelopePersistence(opts Opts) (*EnvelopePersistence, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newEnvelopePersistence(dbs.ReadDB, dbs.WriteDB, tables.Envelope), nil
}

func newEnvelopePersistence(readDB, writeDB *sql.DB, table string) *EnvelopePersistence {
	return &EnvelopePersistence{EnvelopePersistence: common.NewEnvelopePersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
