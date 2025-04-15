/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
)

func mockConfig[T any](config T) *mock.ConfigProvider {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(s string, i interface{}) error {
		*i.(*T) = config
		return nil
	})
	return cp
}
func BenchmarkReadExistingPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	cp := mockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	})
	db, err := NewPersistenceWithOpts("benchmark", cp, NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.ReadExisting(b, db)
}

func BenchmarkReadNonExistingPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()

	cp := mockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	})
	db, err := NewPersistenceWithOpts("benchmark", cp, NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.ReadNonExisting(b, db)
}

func BenchmarkWriteOnePostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()

	cp := mockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	})
	db, err := NewPersistenceWithOpts("benchmark", cp, NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.WriteOne(b, db)
}

func BenchmarkWriteManyPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	cp := mockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	})
	db, err := NewPersistenceWithOpts("benchmark", cp, NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.WriteMany(b, db)
}

func BenchmarkWriteManyPostgresWithIdle(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	cp := mockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
		MaxIdleConns: common.CopyPtr(50),
	})
	db, err := NewPersistenceWithOpts("benchmark", cp, NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.WriteParallel(b, db)
}
