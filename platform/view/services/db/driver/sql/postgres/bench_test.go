/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

func BenchmarkReadExistingPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	db, err := common.NewPersistenceWithOpts[DbOpts]("benchmark", newDbOpts(pgConnStr, 2), NewUnversionedPersistence)
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
	db, err := common.NewPersistenceWithOpts[DbOpts]("benchmark", newDbOpts(pgConnStr, 2), NewUnversionedPersistence)
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
	db, err := common.NewPersistenceWithOpts[DbOpts]("benchmark", newDbOpts(pgConnStr, 2), NewUnversionedPersistence)
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
	db, err := common.NewPersistenceWithOpts[DbOpts]("benchmark", newDbOpts(pgConnStr, 2), NewUnversionedPersistence)
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
	db, err := common.NewPersistenceWithOpts[DbOpts]("benchmark", newDbOpts(pgConnStr, 50), NewUnversionedPersistence)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	common.WriteParallel(b, db)
}

func newDbOpts(pgConnStr string, maxIdleConns int) testOpts {
	return testOpts{dataSource: pgConnStr, maxOpenConns: 50, maxIdleConns: maxIdleConns, maxIdleTime: time.Minute}
}
