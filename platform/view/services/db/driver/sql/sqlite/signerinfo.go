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

type SignerInfoPersistence struct {
	*common.SignerInfoPersistence
}

func NewSignerInfoPersistence(opts common.Opts, table string) (*SignerInfoPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newSignerInfoPersistence(readDB, newRetryWriteDB(writeDB), table), nil
}

func newSignerInfoPersistence(readDB *sql.DB, writeDB common.WriteDB, table string) *SignerInfoPersistence {
	return &SignerInfoPersistence{SignerInfoPersistence: common.NewSignerInfoPersistence(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
