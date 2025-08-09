/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopology_SetLogging(t *testing.T) {
	tests := []struct {
		name         string
		initial      *Logging
		spec         string
		format       string
		wantSpec     string
		wantFormat   string
		wantSanitize bool
	}{
		{
			name:         "Spec and Format provided",
			initial:      &Logging{Spec: "old-spec", Format: "old-format", OtelSanitize: true},
			spec:         "new-spec",
			format:       "new-format",
			wantSpec:     "new-spec",
			wantFormat:   "new-format",
			wantSanitize: true,
		},
		{
			name:         "Only Spec provided",
			initial:      &Logging{Spec: "old-spec", Format: "old-format", OtelSanitize: false},
			spec:         "new-spec",
			format:       "",
			wantSpec:     "new-spec",
			wantFormat:   "old-format",
			wantSanitize: false,
		},
		{
			name:         "Only Format provided",
			initial:      &Logging{Spec: "old-spec", Format: "old-format", OtelSanitize: true},
			spec:         "",
			format:       "new-format",
			wantSpec:     "old-spec",
			wantFormat:   "new-format",
			wantSanitize: true,
		},
		{
			name:         "Neither Spec nor Format provided",
			initial:      &Logging{Spec: "old-spec", Format: "old-format", OtelSanitize: false},
			spec:         "",
			format:       "",
			wantSpec:     "old-spec",
			wantFormat:   "old-format",
			wantSanitize: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			top := &Topology{Logging: tt.initial}

			top.SetLogging(tt.spec, tt.format)

			assert.Equal(t, tt.wantSpec, top.Logging.Spec, "Spec mismatch")
			assert.Equal(t, tt.wantFormat, top.Logging.Format, "Format mismatch")
			assert.Equal(t, tt.wantSanitize, top.Logging.OtelSanitize, "OtelSanitize mismatch")
		})
	}
}
