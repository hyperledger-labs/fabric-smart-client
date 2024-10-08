/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
)

func BenchmarkReadExistingPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	db, err := initPersistence(NewVersioned, pgConnStr, "benchmark", 50)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	dbtest.ReadExisting(b, db)
}

func BenchmarkReadNonExistingPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	db, err := initPersistence(NewVersioned, pgConnStr, "benchmark", 50)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	dbtest.ReadNonExisting(b, db)
}

func BenchmarkWriteOnePostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	db, err := initPersistence(NewVersioned, pgConnStr, "benchmark", 50)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	dbtest.WriteOne(b, db)
}

func BenchmarkWriteManyPostgres(b *testing.B) {
	terminate, pgConnStr, err := StartPostgres(b, false)
	if err != nil {
		b.Fatal(err)
	}
	defer terminate()
	db, err := initPersistence(NewVersioned, pgConnStr, "benchmark", 50)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	dbtest.WriteMany(b, db)
}
