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

const (
	precision = float64(time.Millisecond)
	sigDigits = 6 // Number of significant digits for rounding bucket values
)

// ExponentialBucketTimeRange creates a bucket set for a histogram
// that has exponentially increasing intervals between the values, e.g. 0, 0.5, 1, 2, 4, 8, ...
// Fixed to guarantee exactly 'buckets' number of buckets and produce clean floating-point values.
func ExponentialBucketTimeRange(start, end time.Duration, buckets int) []float64 {
	if buckets <= 1 {
		return []float64{roundToSignificantDigits(start.Seconds())}
	}

	interval := end - start
	if interval <= 0 {
		return []float64{roundToSignificantDigits(start.Seconds())}
	}

	// Calculate factor more safely using Pow instead of Exp(Log(...))
	// This ensures we generate exactly 'buckets' number of buckets
	factor := math.Pow(float64(interval)/precision, 1.0/float64(buckets-1))

	bs := make([]float64, 0, buckets)
	bs = append(bs, roundToSignificantDigits(start.Seconds()))

	// Generate exactly buckets-1 additional buckets
	for i := 1; i < buckets; i++ {
		v := time.Duration(math.Pow(factor, float64(i)) * precision)
		if v > interval {
			v = interval
		}
		// Round to sigDigits significant digits to avoid ugly floating-point representations
		bs = append(bs, roundToSignificantDigits((start + v).Seconds()))
	}

	return bs
}

// roundToSignificantDigits rounds a float64 to sigDigits significant digits
// This produces cleaner values for Prometheus metrics (e.g., 0.001 instead of 0.0009999999999999999)
func roundToSignificantDigits(value float64) float64 {
	if value == 0 {
		return 0
	}

	// Determine the order of magnitude
	magnitude := math.Floor(math.Log10(math.Abs(value)))

	// Calculate the scaling factor to get sigDigits significant figures
	scale := math.Pow(10, float64(sigDigits-1)-magnitude)

	// Round to significant digits
	rounded := math.Round(value*scale) / scale

	// Additional cleanup: round to remove floating-point artifacts
	// This handles cases like 0.0016681000000000001 -> 0.0016681
	if rounded != 0 {
		// Determine decimal places needed
		decimalPlaces := int(math.Max(0, float64(sigDigits-1)-magnitude))
		if decimalPlaces > 0 && decimalPlaces < 15 {
			multiplier := math.Pow(10, float64(decimalPlaces))
			rounded = math.Round(rounded*multiplier) / multiplier
		}
	}

	return rounded
}
