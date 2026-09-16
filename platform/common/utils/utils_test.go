/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMust(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantPanic bool
	}{
		{
			name:      "NoError",
			err:       nil,
			wantPanic: false,
		},
		{
			name:      "WithError",
			err:       errors.New("panic"),
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantPanic {
				require.Panics(t, func() { Must(tt.err) })
			} else {
				require.NotPanics(t, func() { Must(tt.err) })
			}
		})
	}
}

func TestMustGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		val       int
		err       error
		wantPanic bool
	}{
		{
			name:      "NoError",
			val:       10,
			err:       nil,
			wantPanic: false,
		},
		{
			name:      "WithError",
			val:       10,
			err:       errors.New("panic"),
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantPanic {
				require.Panics(t, func() { MustGet(tt.val, tt.err) })
			} else {
				require.Equal(t, tt.val, MustGet(tt.val, tt.err))
			}
		})
	}
}
