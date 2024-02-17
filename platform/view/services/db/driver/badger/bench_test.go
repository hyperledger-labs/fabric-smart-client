/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"path/filepath"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	"github.com/stretchr/testify/assert"
)

func BenchmarkReadExisting(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	assert.NoError(b, err)
	assert.NotNil(b, db)
	defer db.Close()

	dbtest.ReadExisting(b, db)
}

func BenchmarkReadNonExisting(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	assert.NoError(b, err)
	assert.NotNil(b, db)
	defer db.Close()

	dbtest.ReadNonExisting(b, db)
}

func BenchmarkWriteOne(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	assert.NoError(b, err)
	assert.NotNil(b, db)
	defer db.Close()

	dbtest.WriteOne(b, db)
}

func BenchmarkWriteMany(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	assert.NoError(b, err)
	assert.NotNil(b, db)
	defer db.Close()

	dbtest.WriteMany(b, db)
}
