/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJoin tests the Join function
func TestJoin(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{
			name:       "Multiple components",
			components: []string{"server", "ports", "http"},
			expected:   "server.ports.http",
		},
		{
			name:       "Single component",
			components: []string{"version"},
			expected:   "version",
		},
		{
			name:       "No components",
			components: []string{},
			expected:   "",
		},
		{
			name:       "Components with leading/trailing dots",
			components: []string{".app", "settings."},
			expected:   "app.settings",
		},
		{
			name:       "Components with internal dots (should not be split)",
			components: []string{"my.service", "config"},
			expected:   "my.service.config",
		},
		{
			name:       "Mixed case with empty strings",
			components: []string{"parent", "", "child", "grandchild"},
			expected:   "parent.child.grandchild",
		},
		{
			name:       "All empty strings",
			components: []string{"", "", ""},
			expected:   "",
		},
		{
			name:       "Components with spaces and dots",
			components: []string{" user name ", ".api.v1.", "endpoint "},
			expected:   "user name.api.v1.endpoint",
		},
		{
			name:       "Complex case with various trims",
			components: []string{" .key1 ", " key2. ", " key3 ", ".key4."},
			expected:   "key1.key2.key3.key4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Join(tt.components...)
			assert.Equal(t, tt.expected, got, "Join(%v) = %q; want %q", tt.components, got, tt.expected)
		})
	}
}
