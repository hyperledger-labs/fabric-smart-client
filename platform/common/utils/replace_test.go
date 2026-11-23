/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceLongestFirst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		s            string
		replacements map[string]string
		expected     string
	}{
		{name: "no replacements", s: "a.b.c", expected: "a.b.c"},
		{name: "a key that does not match", s: "a.b.c", replacements: map[string]string{"x": "y"}, expected: "a.b.c"},
		{name: "every occurrence", s: "a.b.a", replacements: map[string]string{"a": "x"}, expected: "x.b.x"},
		{
			name:         "the longest of overlapping keys wins",
			s:            "a.b.c.d",
			replacements: map[string]string{"a.b": "short", "a.b.c": "long"},
			expected:     "long.d",
		},
		{
			name:         "a match that starts earlier wins over a longer one that starts later",
			s:            "x.a.b.c",
			replacements: map[string]string{"x.a": "S", "a.b.c": "L"},
			expected:     "S.b.c",
		},
		{
			name:         "a replaced part is not scanned again",
			s:            "a.b",
			replacements: map[string]string{"a": "b", "b": "c"},
			expected:     "b.c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// A map ranges in a different order on each call, so repeat to catch a
			// result that depends on it.
			for range 100 {
				assert.Equal(t, tc.expected, ReplaceLongestFirst(tc.s, tc.replacements))
			}
		})
	}
}
