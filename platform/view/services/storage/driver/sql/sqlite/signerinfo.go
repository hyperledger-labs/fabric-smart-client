/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type SignerInfoStore struct {
	*sqlcommon.SignerInfoStore
}

func NewSignerInfoStore(dbs *drivercommon.RWDB, tables sqlcommon.TableNames) (*SignerInfoStore, error) {
	return buildSignerInfoStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.SignerInfo), nil
}

func buildSignerInfoStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, table string) *SignerInfoStore {
	return &SignerInfoStore{SignerInfoStore: sqlcommon.NewSignerInfoStore(writeDB, readDB, table, &ErrorMapper{})}
}
