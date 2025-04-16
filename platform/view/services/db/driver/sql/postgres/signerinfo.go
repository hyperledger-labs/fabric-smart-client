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

type SignerInfoPersistence struct {
	*common.SignerInfoPersistence
}

func NewSignerInfoPersistence(opts Opts) (*SignerInfoPersistence, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newSignerInfoPersistence(dbs.ReadDB, dbs.WriteDB, tables.SignerInfo), nil
}

func newSignerInfoPersistence(readDB, writeDB *sql.DB, table string) *SignerInfoPersistence {
	return &SignerInfoPersistence{SignerInfoPersistence: common.NewSignerInfoPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
