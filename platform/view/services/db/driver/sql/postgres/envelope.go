/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type EnvelopeStore struct {
	*common.EnvelopeStore
}

func NewEnvelopeStore(dbs *common2.RWDB, tables common.TableNames) (*EnvelopeStore, error) {
	return newEnvelopeStore(dbs.ReadDB, dbs.WriteDB, tables.Envelope), nil
}

func newEnvelopeStore(readDB, writeDB *sql.DB, table string) *EnvelopeStore {
	return &EnvelopeStore{EnvelopeStore: common.NewEnvelopeStore(readDB, writeDB, table, &errorMapper{}, NewConditionInterpreter())}
}
