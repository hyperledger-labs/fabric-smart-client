/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
)

func TestEnvelope_GetData(t *testing.T) { //nolint:paralleltest
	GetData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEnvelopeStore(db).GetEnvelope(context.Background(), key)
	})
}

func TestEnvelope_GetData_NoData(t *testing.T) { //nolint:paralleltest
	GetDataNoData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEnvelopeStore(db).GetEnvelope(context.Background(), key)
	})
}

func TestEnvelope_ExistData_True(t *testing.T) { //nolint:paralleltest
	ExistDataTrue(t, func(db *sql.DB, key string) (bool, error) {
		return mockEnvelopeStore(db).ExistsEnvelope(context.Background(), key)
	})
}

func TestEnvelope_ExistData_False(t *testing.T) { //nolint:paralleltest
	ExistDataFalse(t, func(db *sql.DB, key string) (bool, error) {
		return mockEnvelopeStore(db).ExistsEnvelope(context.Background(), key)
	})
}

func TestEnvelope_PutData_Success(t *testing.T) { //nolint:paralleltest
	PutDataSuccess(t, func(db *sql.DB, key string, data []byte) error {
		return mockEnvelopeStore(db).PutEnvelope(context.Background(), key, data)
	})
}

func TestEnvelope_PutData_Conflict(t *testing.T) { //nolint:paralleltest
	PutDataConflict(t, func(db *sql.DB, key string, data []byte) error {
		return mockEnvelopeStore(db).PutEnvelope(context.Background(), key, data)
	})
}

func mockEnvelopeStore(db *sql.DB) *common.EnvelopeStore {
	return common.NewEnvelopeStore(db, db, "test_table", &mock.SQLErrorWrapper{})
}
