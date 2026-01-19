/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// Test Fix #1: Safer calculation method (Pow vs Exp/Log) - maintains exponential spacing
func TestFix1_SaferCalculation_MaintainsExponentialSpacing(t *testing.T) {
	t.Run("Verify exponential spacing with constant ratio", func(t *testing.T) {
		buckets := ExponentialBucketTimeRange(0, 1*time.Second, 10)

		// Calculate ratios between consecutive buckets
		var ratios []float64
		for i := 2; i < len(buckets); i++ {
			if buckets[i-1] > 0 {
				ratio := buckets[i] / buckets[i-1]
				ratios = append(ratios, ratio)
			}
		}

		// All ratios should be approximately equal (exponential spacing)
		if len(ratios) < 2 {
			t.Fatal("Not enough ratios to verify exponential spacing")
		}

		expectedRatio := ratios[0]
		for i, ratio := range ratios {
			diff := math.Abs(ratio - expectedRatio)
			if diff > 0.01 { // Allow 1% tolerance
				t.Errorf("Ratio %d: %.6f differs from expected %.6f by %.6f", i, ratio, expectedRatio, diff)
			}
		}

		t.Logf("✓ Exponential spacing confirmed: constant ratio = %.6f", expectedRatio)
		t.Logf("  Buckets: %v", buckets)
	})
}

// Test Fix #2: Guaranteed exact bucket count
func TestFix2_GuaranteedExactBucketCount(t *testing.T) {
	testCases := []struct {
		name    string
		start   time.Duration
		end     time.Duration
		buckets int
	}{
		{"10 buckets", 0, 1 * time.Second, 10},
		{"15 buckets", 0, 5 * time.Second, 15},
		{"7 buckets", 0, 1 * time.Second, 7},
		{"2 buckets", 0, 1 * time.Second, 2},
		{"20 buckets", 0, 10 * time.Second, 20},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExponentialBucketTimeRange(tc.start, tc.end, tc.buckets)

			if len(result) != tc.buckets {
				t.Errorf("Expected exactly %d buckets, got %d", tc.buckets, len(result))
				t.Errorf("Buckets: %v", result)
			} else {
				t.Logf("✓ Got exactly %d buckets as requested", tc.buckets)
			}
		})
	}
}

// Test Fix #3: Independent calculation (no error accumulation)
func TestFix3_IndependentCalculation_NoErrorAccumulation(t *testing.T) {
	t.Run("Verify monotonically increasing values", func(t *testing.T) {
		buckets := ExponentialBucketTimeRange(0, 5*time.Second, 15)

		for i := 1; i < len(buckets); i++ {
			if buckets[i] <= buckets[i-1] {
				t.Errorf("Bucket %d (%.10f) is not greater than bucket %d (%.10f)",
					i, buckets[i], i-1, buckets[i-1])
			}
		}

		t.Logf("✓ All buckets monotonically increasing")
	})

	t.Run("Verify first and last buckets match start and end", func(t *testing.T) {
		start := 0 * time.Second
		end := 1 * time.Second
		buckets := ExponentialBucketTimeRange(start, end, 10)

		if buckets[0] != start.Seconds() {
			t.Errorf("First bucket %.10f != start %.10f", buckets[0], start.Seconds())
		}

		// Last bucket should be close to end (within rounding)
		diff := math.Abs(buckets[len(buckets)-1] - end.Seconds())
		if diff > 0.01 {
			t.Errorf("Last bucket %.10f differs from end %.10f by %.10f",
				buckets[len(buckets)-1], end.Seconds(), diff)
		}

		t.Logf("✓ First bucket = %.10f (start)", buckets[0])
		t.Logf("✓ Last bucket = %.10f (end = %.10f)", buckets[len(buckets)-1], end.Seconds())
	})
}

// Test Fix #4: Rounding to significant digits produces clean values
func TestFix4_RoundingProducesCleanValues(t *testing.T) {
	t.Run("Verify Prometheus output format is clean", func(t *testing.T) {
		buckets := ExponentialBucketTimeRange(0, 1*time.Second, 10)

		for i, v := range buckets {
			// Format as Prometheus would (%g format)
			prometheusStr := fmt.Sprintf("%g", v)

			// Check for floating point issue patterns
			if len(prometheusStr) > 12 && v > 0 && v < 10 {
				t.Errorf("Bucket %d has potentially floating point issue in Prometheus output: le=\"%s\"", i, prometheusStr)
			}

			t.Logf("Bucket %d: le=\"%s\" ✓", i, prometheusStr)
		}
	})

	t.Run("Compare internal vs Prometheus representation", func(t *testing.T) {
		buckets := ExponentialBucketTimeRange(0, 100*time.Millisecond, 10)

		t.Logf("\nInternal vs Prometheus representation:")
		for i, v := range buckets {
			internal := fmt.Sprintf("%.17g", v)
			prometheus := fmt.Sprintf("%g", v)
			t.Logf("  Bucket %d: internal=%.17g, prometheus=le=\"%s\"", i, v, prometheus)

			// Prometheus format should be shorter/cleaner
			if len(prometheus) > len(internal) {
				t.Errorf("Prometheus format longer than internal for bucket %d", i)
			}
		}
	})
}

// Test Fix #5: Edge case handling
func TestFix5_EdgeCaseHandling(t *testing.T) {
	testCases := []struct {
		name           string
		start          time.Duration
		end            time.Duration
		buckets        int
		expectedLength int
		shouldPanic    bool
	}{
		{"Zero buckets", 0, 1 * time.Second, 0, 1, false},
		{"Negative buckets", 0, 1 * time.Second, -5, 1, false},
		{"Single bucket", 0, 1 * time.Second, 1, 1, false},
		{"Start equals end", 1 * time.Second, 1 * time.Second, 10, 1, false},
		{"Start greater than end", 5 * time.Second, 1 * time.Second, 10, 1, false},
		{"Normal case", 0, 1 * time.Second, 10, 10, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tc.shouldPanic {
						t.Errorf("Unexpected panic: %v", r)
					}
				}
			}()

			result := ExponentialBucketTimeRange(tc.start, tc.end, tc.buckets)

			if len(result) != tc.expectedLength {
				t.Errorf("Expected %d buckets, got %d", tc.expectedLength, len(result))
			} else {
				t.Logf("✓ Handled edge case correctly: got %d bucket(s)", len(result))
			}

			t.Logf("  Result: %v", result)
		})
	}
}

// Comprehensive test combining all fixes
func TestAllFixes_Comprehensive(t *testing.T) {
	t.Run("10 buckets from 0 to 1 second", func(t *testing.T) {
		buckets := ExponentialBucketTimeRange(0, 1*time.Second, 10)

		// Fix #2: Exact count
		if len(buckets) != 10 {
			t.Errorf("Expected 10 buckets, got %d", len(buckets))
		}

		// Fix #3: Monotonic
		for i := 1; i < len(buckets); i++ {
			if buckets[i] <= buckets[i-1] {
				t.Errorf("Not monotonic at index %d", i)
			}
		}

		// Fix #1: Exponential spacing
		var ratios []float64
		for i := 2; i < len(buckets); i++ {
			if buckets[i-1] > 0 {
				ratios = append(ratios, buckets[i]/buckets[i-1])
			}
		}

		if len(ratios) > 1 {
			avgRatio := ratios[0]
			for _, r := range ratios {
				if math.Abs(r-avgRatio) > 0.01 {
					t.Errorf("Ratio variance too high: %.6f vs %.6f", r, avgRatio)
				}
			}
		}

		// Fix #4: Clean Prometheus output
		for i, v := range buckets {
			prometheusStr := fmt.Sprintf("%g", v)
			if len(prometheusStr) > 12 && v > 0 && v < 10 {
				t.Errorf("Bucket %d has potentially floating point issue in the output: %s", i, prometheusStr)
			}
		}

		t.Logf("✓ All fixes verified")
		t.Logf("  Buckets: %v", buckets)
		t.Logf("  Exponential ratio: %.6f", ratios[0])
	})
}

// Test for LinearBucketTimeRange (unchanged, for completeness)
func TestLinearBucketTimeRange(t *testing.T) {
	tests := []struct {
		name    string
		start   time.Duration
		end     time.Duration
		buckets int
	}{
		{"10 buckets from 0 to 1 second", 0, 1 * time.Second, 10},
		{"5 buckets from 0 to 5 seconds", 0, 5 * time.Second, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buckets := LinearBucketTimeRange(tt.start, tt.end, tt.buckets)

			expectedLen := tt.buckets + 1
			if len(buckets) != expectedLen {
				t.Errorf("Expected %d buckets, got %d", expectedLen, len(buckets))
			}

			if buckets[0] != tt.start.Seconds() {
				t.Errorf("First bucket = %v, want %v", buckets[0], tt.start.Seconds())
			}

			if math.Abs(buckets[len(buckets)-1]-tt.end.Seconds()) > 0.0001 {
				t.Errorf("Last bucket = %v, want %v", buckets[len(buckets)-1], tt.end.Seconds())
			}

			t.Logf("Linear buckets: %v", buckets)
		})
	}
}

// Test for LinearBucketRange (unchanged, for completeness)
func TestLinearBucketRange(t *testing.T) {
	tests := []struct {
		name    string
		start   int64
		end     int64
		buckets int
	}{
		{"10 buckets from 0 to 100", 0, 100, 10},
		{"5 buckets from 10 to 50", 10, 50, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buckets := LinearBucketRange(tt.start, tt.end, tt.buckets)

			expectedLen := tt.buckets + 1
			if len(buckets) != expectedLen {
				t.Errorf("Expected %d buckets, got %d", expectedLen, len(buckets))
			}

			if buckets[0] != float64(tt.start) {
				t.Errorf("First bucket = %v, want %v", buckets[0], float64(tt.start))
			}

			if math.Abs(buckets[len(buckets)-1]-float64(tt.end)) > 0.0001 {
				t.Errorf("Last bucket = %v, want %v", buckets[len(buckets)-1], float64(tt.end))
			}

			t.Logf("Linear range buckets: %v", buckets)
		})
	}
}
