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

func NewSignerInfoPersistence(opts DbOpts, table string) (*SignerInfoPersistence, error) {
	readWriteDB, err := openDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newSignerInfoPersistence(readWriteDB, readWriteDB, table), nil
}

func newSignerInfoPersistence(readDB, writeDB *sql.DB, table string) *SignerInfoPersistence {
	return &SignerInfoPersistence{SignerInfoPersistence: common.NewSignerInfoPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
