/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterPrintable(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		prefix   string
		expected string
	}{
		{
			name:     "no non-printable chars",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "with null byte",
			input:    "hello\x00world",
			expected: "[nonprintable] helloworld",
		},
		{
			name:     "with tab character",
			input:    "go\tlang",
			expected: "[nonprintable] golang",
		},
		{
			name:     "with bell character",
			input:    "warn\x07ing",
			expected: "[nonprintable] warning",
		},
		{
			name:     "only printable",
			input:    "12345",
			expected: "12345",
		},
		{
			name:     "only non-printable",
			input:    "\x00\x07\x1B",
			expected: "[nonprintable] ",
		},
		{
			name:     "with null byte + prefix check",
			input:    "hello\x00world 12345678901234567890",
			expected: "[nonprintable] helloworld 12345678901234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterPrintableWithMarker(tt.input)
			assert.Equal(t, tt.expected, got, "got %q, want %q", got, tt.expected)

			// check Printable
			assert.Equal(t, strings.TrimPrefix(got, "[nonprintable] "), Printable(tt.input).String())

			// check Prefix
			prefixStr := Prefix(tt.input).String()
			assert.Equal(t, Prefix(Printable(tt.input).String()).String(), prefixStr)
			assert.Equal(t, prefixStr, Printable(prefixStr).String())
		})
	}
}
