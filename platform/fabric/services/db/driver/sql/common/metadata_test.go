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

func TestMetadata_GetData(t *testing.T) {
	t.Parallel()
	GetData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockMetadataStore(db).GetMetadata(context.Background(), key)
	})
}

func TestMetadata_GetData_NoData(t *testing.T) {
	t.Parallel()
	GetDataNoData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockMetadataStore(db).GetMetadata(context.Background(), key)
	})
}

func TestMetadata_ExistData_True(t *testing.T) {
	t.Parallel()
	ExistDataTrue(t, func(db *sql.DB, key string) (bool, error) {
		return mockMetadataStore(db).ExistMetadata(context.Background(), key)
	})
}

func TestMetadata_ExistData_False(t *testing.T) {
	t.Parallel()
	ExistDataFalse(t, func(db *sql.DB, key string) (bool, error) {
		return mockMetadataStore(db).ExistMetadata(context.Background(), key)
	})
}

func TestMetadata_PutData_Success(t *testing.T) {
	t.Parallel()
	PutDataSuccess(t, func(db *sql.DB, key string, data []byte) error {
		return mockMetadataStore(db).PutMetadata(context.Background(), key, data)
	})
}

func TestMetadata_PutData_Conflict(t *testing.T) {
	t.Parallel()
	PutDataConflict(t, func(db *sql.DB, key string, data []byte) error {
		return mockMetadataStore(db).PutMetadata(context.Background(), key, data)
	})
}

func mockMetadataStore(db *sql.DB) *common.MetadataStore {
	return common.NewMetadataStore(db, db, "test_table", &mock.SQLErrorWrapper{})
}
