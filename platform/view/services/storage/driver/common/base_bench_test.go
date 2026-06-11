/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"
)

func BenchmarkBaseDB_BeginUpdate(b *testing.B) {
	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}
	db := NewBaseDB(newTxn)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = db.BeginUpdate()
		_ = db.Discard()
	}
}

func BenchmarkBaseDB_Commit(b *testing.B) {
	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}
	db := NewBaseDB(newTxn)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = db.BeginUpdate()
		_ = db.Commit()
	}
}

func BenchmarkBaseDB_Discard(b *testing.B) {
	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}
	db := NewBaseDB(newTxn)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = db.BeginUpdate()
		_ = db.Discard()
	}
}
