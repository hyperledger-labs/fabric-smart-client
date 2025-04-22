/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

func BenchmarkReadExistingPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	cp := NewConfigProvider(common2.MockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	}))
	db, err := NewPersistenceWithOpts(cp, "", NewUnversionedPersistence)
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

	cp := NewConfigProvider(common2.MockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	}))
	db, err := NewPersistenceWithOpts(cp, "", NewUnversionedPersistence)
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

	cp := NewConfigProvider(common2.MockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	}))
	db, err := NewPersistenceWithOpts(cp, "", NewUnversionedPersistence)
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
	cp := NewConfigProvider(common2.MockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	}))
	db, err := NewPersistenceWithOpts(cp, "", NewUnversionedPersistence)
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
	cp := NewConfigProvider(common2.MockConfig(Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
		MaxIdleConns: common2.CopyPtr(50),
	}))
	db, err := NewPersistenceWithOpts(cp, "", NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.WriteParallel(b, db)
}
