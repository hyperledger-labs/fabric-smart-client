/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
)

func BenchmarkUUID(b *testing.B) {
	// generateUUIDv1 is our reference impl for rand UUID based on previous version of this code
	oldGenerateUUID := func() string {
		uuid := make([]byte, 16)

		_, err := io.ReadFull(rand.Reader, uuid[:])
		if err != nil {
			panic(fmt.Sprintf("Error generating UUID: %s", err))
		}

		// variant bits; see section 4.1.1
		uuid[8] = uuid[8]&^0xc0 | 0x80

		// version 4 (pseudo-random); see section 4.1.3
		uuid[6] = uuid[6]&^0xf0 | 0x40

		id := uuid

		return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:])
	}

	b.Run("oldGenerateUUID", func(b *testing.B) {
		report(b)
		for b.Loop() {
			_ = oldGenerateUUID()
		}
	})

	b.Run("GenerateBytesUUID", func(b *testing.B) {
		report(b)
		for b.Loop() {
			_ = GenerateBytesUUID()
		}
	})

	b.Run("GenerateUUID", func(b *testing.B) {
		report(b)
		for b.Loop() {
			_ = GenerateUUID()
		}
	})

	b.Run("GenerateUUID (non-pooled)", func(b *testing.B) {
		report(b)
		uuid.DisableRandPool()
		for b.Loop() {
			_ = GenerateUUID()
		}
	})

	b.Run("GenerateUUID parallel", func(b *testing.B) {
		report(b)
		uuid.EnableRandPool()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = GenerateUUID()
			}
		})
	})

	b.Run("GenerateUUID parallel (non-pooled)", func(b *testing.B) {
		report(b)
		uuid.DisableRandPool()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = GenerateUUID()
			}
		})
	})
}

func report(b *testing.B) {
	b.ReportAllocs()
}
