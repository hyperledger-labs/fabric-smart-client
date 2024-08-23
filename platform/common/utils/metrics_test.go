/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLinearBucketRange(t *testing.T) {
	buckets := LinearBucketRange(0, 5*time.Second, 10)
	assert.Equal(t, []float64{0, 0.5, 1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5}, buckets)
}

func TestExponentialBucketRange(t *testing.T) {
	buckets := ExponentialBucketRange(0, 1*time.Second, 10)
	assert.Equal(t, []float64{0, 0.002154434, 0.004641588, 0.01, 0.021544346, 0.046415888, 0.1, 0.215443469, 0.464158883, 1}, buckets)
}
