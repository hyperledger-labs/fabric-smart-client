/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type EnvelopeStore struct {
	*common.EnvelopeStore
}

func NewEnvelopeStore(dbs *common3.RWDB, tables common.TableNames) (*EnvelopeStore, error) {
	return newEnvelopeStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Envelope), nil
}

func newEnvelopeStore(readDB *sql.DB, writeDB common.WriteDB, table string) *EnvelopeStore {
	return &EnvelopeStore{EnvelopeStore: common.NewEnvelopeStore(writeDB, readDB, table, &errorMapper{}, NewConditionInterpreter())}
}
