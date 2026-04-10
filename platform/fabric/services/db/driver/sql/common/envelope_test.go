/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common/mock"
)

func TestEndorseTX_GetData(t *testing.T) { //nolint:paralleltest
	GetData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEndorseTXStore(db).GetEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_GetData_NoData(t *testing.T) { //nolint:paralleltest
	GetData_NoData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEndorseTXStore(db).GetEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_ExistData_True(t *testing.T) { //nolint:paralleltest
	ExistData_True(t, func(db *sql.DB, key string) (bool, error) {
		return mockEndorseTXStore(db).ExistsEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_ExistData_False(t *testing.T) { //nolint:paralleltest
	ExistData_False(t, func(db *sql.DB, key string) (bool, error) {
		return mockEndorseTXStore(db).ExistsEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_PutData_Success(t *testing.T) { //nolint:paralleltest
	PutData_Success(t, func(db *sql.DB, key string, data []byte) error {
		return mockEndorseTXStore(db).PutEndorseTx(context.Background(), key, data)
	})
}

func TestEndorseTX_PutData_Conflict(t *testing.T) { //nolint:paralleltest
	PutData_Conflict(t, func(db *sql.DB, key string, data []byte) error {
		return mockEndorseTXStore(db).PutEndorseTx(context.Background(), key, data)
	})
}

func mockEndorseTXStore(db *sql.DB) *common.EndorseTxStore {
	return common.NewEndorseTxStore(db, db, "test_table", &mock.SQLErrorWrapper{}, sq.Dollar)
}
