/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	returnValue []byte
	returnErr   error
	payload     = []byte("hallo")
)

func BenchmarkReadExisting(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(b, err)
	assert.NotNil(b, db)

	db.BeginUpdate()
	db.SetState(namespace, key, payload, 0, 0)
	db.Commit()

	var v []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, _, _, _ = db.GetState(namespace, key)
	}
	b.StopTimer()
	returnValue = v
	assert.NotNil(b, returnValue)
}

func BenchmarkReadNonExisting(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(b, err)
	assert.NotNil(b, db)

	var v []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, _, _, _ = db.GetState(namespace, key)
	}
	b.StopTimer()
	returnValue = v
	assert.NotNil(b, returnValue)
}

func BenchmarkWriteOne(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(b, err)
	assert.NotNil(b, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = db.BeginUpdate()
		_ = err
		err = db.SetState(namespace, key, payload, 0, 0)
		_ = err
		err = db.Commit()
	}
	b.StopTimer()
	returnErr = err
	assert.NoError(b, returnErr)
}

func BenchmarkWriteMany(b *testing.B) {
	dbpath := filepath.Join(tempDir, "badger-benchmark")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(b, err)
	assert.NotNil(b, db)

	var k string
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k = fmt.Sprintf("key_%d", i)

		err = db.BeginUpdate()
		_ = err
		err = db.SetState(namespace, k, payload, 0, 0)
		_ = err
		err = db.Commit()
	}
	b.StopTimer()
	returnErr = err
	assert.NoError(b, returnErr)
}
