/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dbtest

import (
	"fmt"
	"math"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/stretchr/testify/assert"
)

var (
	returnValue []byte
	returnErr   error
	payload     = []byte("hallo")
)

func ReadExisting(b *testing.B, db driver.TransactionalVersionedPersistence) {
	assert.NoError(b, db.BeginUpdate())
	assert.NoError(b, db.SetState(namespace, key, driver.VersionedValue{Raw: payload}))
	assert.NoError(b, db.Commit())

	var v []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vv, _ := db.GetState(namespace, key)
		v = vv.Raw
	}
	b.StopTimer()
	returnValue = v
	assert.NotNil(b, returnValue)
	b.Logf("%.0f reads per second from same key", float64(b.N)/b.Elapsed().Seconds())
}

func ReadNonExisting(b *testing.B, db driver.TransactionalVersionedPersistence) {
	var v []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vv, _ := db.GetState(namespace, key)
		v = vv.Raw
	}
	b.StopTimer()
	returnValue = v
	assert.Nil(b, returnValue)
	b.Logf("%.0f reads per second to nonexistent keys", float64(b.N)/b.Elapsed().Seconds())
}

func WriteOne(b *testing.B, db driver.TransactionalVersionedPersistence) {
	var err error
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = db.BeginUpdate()
		_ = err
		err = db.SetState(namespace, key, driver.VersionedValue{Raw: payload})
		_ = err
		err = db.Commit()
	}
	b.StopTimer()
	returnErr = err
	assert.NoError(b, returnErr)
	b.Logf("%.0f writes per second to same key", float64(b.N)/b.Elapsed().Seconds())
}

func WriteMany(b *testing.B, db driver.TransactionalVersionedPersistence) {
	var err error
	var k string
	b.Logf("before: %+v", db.Stats())
	mid := math.Round(float64(b.N) / 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k = fmt.Sprintf("key_%d", i)

		err = db.BeginUpdate()
		_ = err
		err = db.SetState(namespace, k, driver.VersionedValue{Raw: payload})
		_ = err
		err = db.Commit()

		if i == int(mid) {
			b.Logf("mid: %+v", db.Stats())
		}
	}
	b.StopTimer()
	returnErr = err
	assert.NoError(b, returnErr)
	b.Logf("%.0f writes per second to different keys", float64(b.N)/b.Elapsed().Seconds())
	b.Logf("after: %+v", db.Stats())
}

func WriteParallel(b *testing.B, db driver.TransactionalUnversionedPersistence) {
	var err error
	var k string
	var i int
	b.Logf("    before: %+v", db.Stats())
	mid := math.Round(float64(b.N) / 2)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i++                          // i is not unique because of paralellism but that's ok
			k = fmt.Sprintf("key_%d", i) // this could be optimized by moving the key creation out of the benchmark
			if i == int(mid) {
				b.Logf("    mid (%d): %+v", int(mid), db.Stats())
			}

			tx, err := db.NewWriteTransaction()
			if err != nil {
				b.Error(err)
			}
			if err := tx.SetState(namespace, k, payload); err != nil {
				b.Error(err)
			}
			if err := tx.Commit(); err != nil {
				b.Error(err)
			}
		}
	})

	b.StopTimer()
	returnErr = err
	assert.NoError(b, returnErr)
	b.Logf("    after: %+v", db.Stats())
	b.Logf("%.0f writes per second to different keys", float64(b.N)/b.Elapsed().Seconds())
}
