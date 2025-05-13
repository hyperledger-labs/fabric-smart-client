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

type EnvelopeStore struct {
	*common.EnvelopeStore
}

func NewEnvelopeStore(opts Opts) (*EnvelopeStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newEnvelopeStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Envelope), nil
}

func newEnvelopeStore(readDB *sql.DB, writeDB common.WriteDB, table string) *EnvelopeStore {
	return &EnvelopeStore{EnvelopeStore: common.NewEnvelopeStore(writeDB, readDB, table, &errorMapper{}, NewConditionInterpreter())}
}
