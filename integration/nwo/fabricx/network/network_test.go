/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNamespaceList(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []Namespace
	}{
		{
			name: "single namespace",
			output: `Installed namespaces (1 total):
0) ns1: version 0 policy: AABBCCDDEEFF`,
			expected: []Namespace{
				{
					Name:    "ns1",
					Version: 0,
				},
			},
		},
		{
			name: "multiple namespaces with different versions",
			output: `Installed namespaces (2 total):
0) ns: version 0 policy: AABBCCDDEEFF
0) ns2: version 0 policy: AABBCCDDEEFF
0) ns3: version 1 policy: AABBCCDDEEFF`,
			expected: []Namespace{
				{
					Name:    "ns",
					Version: 0,
				},
				{
					Name:    "ns2",
					Version: 0,
				},
				{
					Name:    "ns3",
					Version: 1,
				},
			},
		},
		{
			name:     "empty namespace list",
			output:   `Installed namespaces (0 total):`,
			expected: []Namespace{},
		},
		{
			name:     "invalid output returns empty list",
			output:   `Error: some error message`,
			expected: []Namespace{},
		},
		{
			name: "invalid version number is skipped with warning",
			output: `Installed namespaces (2 total):
0) ns1: version 0 policy: AABBCCDDEEFF
1) ns2: version invalid policy: AABBCCDDEEFF
2) ns3: version 1 policy: AABBCCDDEEFF`,
			expected: []Namespace{
				{
					Name:    "ns1",
					Version: 0,
				},
				{
					Name:    "ns3",
					Version: 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNamespaceList(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}
