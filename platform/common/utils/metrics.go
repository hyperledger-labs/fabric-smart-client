/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"math"
	"time"
)

// LinearBucketRange creates a bucket set for a histogram
// that has uniform intervals between the values, e.g. 0, 1, 2, 3, 4, ...
func LinearBucketRange(start, end time.Duration, buckets int) []float64 {
	bs := make([]float64, 0, buckets+1)
	step := (end.Seconds() - start.Seconds()) / float64(buckets)
	for v := start.Seconds(); v <= end.Seconds(); v += step {
		bs = append(bs, v)
	}
	return bs
}

const precision = float64(time.Millisecond)

// ExponentialBucketRange creates a bucket set for a histogram
// that has exponentially increasing intervals between the values, e.g. 0, 0.5, 1, 2, 4, 8, ...
func ExponentialBucketRange(start, end time.Duration, buckets int) []float64 {
	interval := end - start
	factor := math.Exp(math.Log(float64(interval)/precision) / float64(buckets-1))
	bs := make([]float64, 0, buckets)
	bs = append(bs, start.Seconds())
	for f, v := factor, time.Duration(factor*precision); v <= interval; f, v = f*factor, time.Duration(f*factor*precision) {
		bs = append(bs, (start + v).Seconds())
	}
	return bs
}
