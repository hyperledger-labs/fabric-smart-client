/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	"github.com/stretchr/testify/assert"
)

func BenchmarkReadExistingSqlite(b *testing.B) {
	db, err := newVersionedPersistence(b.TempDir())
	assert.NoError(b, err)
	defer db.Close()

	dbtest.ReadExisting(b, db)
}

func BenchmarkReadNonExistingSqlite(b *testing.B) {
	db, err := newVersionedPersistence(b.TempDir())
	assert.NoError(b, err)
	defer db.Close()

	dbtest.ReadNonExisting(b, db)
}

func BenchmarkWriteOneSqlite(b *testing.B) {
	db, err := newVersionedPersistence(b.TempDir())
	assert.NoError(b, err)
	defer db.Close()

	dbtest.WriteOne(b, db)
}

func BenchmarkWriteManySqlite(b *testing.B) {
	db, err := newVersionedPersistence(b.TempDir())
	assert.NoError(b, err)
	defer db.Close()

	dbtest.WriteMany(b, db)
}

func newVersionedPersistence(dir string) (*VersionedPersistence, error) {
	p, err := NewVersioned(versionedOpts("benchmark", dir), "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}
