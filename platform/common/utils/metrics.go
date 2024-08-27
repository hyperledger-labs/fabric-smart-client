/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"math"
	"time"
)

// LinearBucketTimeRange creates a bucket set for a histogram
// that has uniform intervals between the values, e.g. 0, 1, 2, 3, 4, ...
func LinearBucketTimeRange(start, end time.Duration, buckets int) []float64 {
	bs := make([]float64, 0, buckets+1)
	step := (end.Seconds() - start.Seconds()) / float64(buckets)
	for v := start.Seconds(); v <= end.Seconds(); v += step {
		bs = append(bs, v)
	}
	return bs
}

// LinearBucketRange creates a bucket set for a histogram
// that has uniform intervals between the values, e.g. 0, 1, 2, 3, 4, ...
func LinearBucketRange(start, end int64, buckets int) []float64 {
	bs := make([]float64, 0, buckets+1)
	step := float64(end-start) / float64(buckets)
	for v := float64(start); v <= float64(end); v += step {
		bs = append(bs, v)
	}
	return bs
}

const precision = float64(time.Millisecond)

// ExponentialBucketTimeRange creates a bucket set for a histogram
// that has exponentially increasing intervals between the values, e.g. 0, 0.5, 1, 2, 4, 8, ...
func ExponentialBucketTimeRange(start, end time.Duration, buckets int) []float64 {
	interval := end - start
	factor := math.Exp(math.Log(float64(interval)/precision) / float64(buckets-1))
	bs := make([]float64, 0, buckets)
	bs = append(bs, start.Seconds())
	for f, v := factor, time.Duration(factor*precision); v <= interval; f, v = f*factor, time.Duration(f*factor*precision) {
		bs = append(bs, (start + v).Seconds())
	}
	return bs
}
