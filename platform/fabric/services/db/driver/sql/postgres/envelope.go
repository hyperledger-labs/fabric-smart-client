/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

type EnvelopeStore struct {
	*common.EnvelopeStore
}

func NewEnvelopeStore(dbs *drivercommon.RWDB, tables common.TableNames) (*EnvelopeStore, error) {
	return buildEnvelopeStore(dbs.ReadDB, dbs.WriteDB, tables.Envelope), nil
}

func buildEnvelopeStore(readDB, writeDB *sql.DB, table string) *EnvelopeStore {
	return &EnvelopeStore{EnvelopeStore: common.NewEnvelopeStore(writeDB, readDB, table, &postgres.ErrorMapper{})}
}
