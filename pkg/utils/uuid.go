/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"github.com/google/uuid"
)

func init() {
	// we enable rand pool for better performance.
	// note that the pooled random bytes are stored in heap;
	// for more details see uuid.EnableRandPool docs.

	// BenchmarkUUID/google_uuid_(pooled)
	// BenchmarkUUID/google_uuid_(pooled)-10      	18897972	        63.14 ns/op	      48 B/op	       1 allocs/op
	// BenchmarkUUID/google_uuid_(non-pooled)
	// BenchmarkUUID/google_uuid_(non-pooled)-10  	 3658857	       326.3 ns/op	      64 B/op	       2 allocs/op
	uuid.EnableRandPool()
}

// GenerateBytesUUID creates a new random UUID and returns it as []byte
func GenerateBytesUUID() []byte {
	u := uuid.New()
	return u[:]
}

// GenerateUUID creates a new random UUID and returns it as a string
func GenerateUUID() string {
	return uuid.NewString()
}
