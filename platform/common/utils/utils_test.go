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

func TestZero(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, Zero[int]())
	require.Empty(t, Zero[string]())
	require.Nil(t, Zero[*int]())
}

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

func TestIsNil(t *testing.T) {
	t.Parallel()

	var m map[string]int
	var s []int
	var c chan int
	var f func()
	i := 10

	tests := []struct {
		name string
		val  any
		want bool
	}{
		{
			name: "NilPointer",
			val:  (*int)(nil),
			want: true,
		},
		{
			name: "NilMap",
			val:  m,
			want: true,
		},
		{
			name: "NilSlice",
			val:  s,
			want: true,
		},
		{
			name: "NilChan",
			val:  c,
			want: true,
		},
		{
			name: "NilFunc",
			val:  f,
			want: true,
		},
		{
			name: "NonNilInt",
			val:  10,
			want: false,
		},
		{
			name: "NonNilString",
			val:  "hello",
			want: false,
		},
		{
			name: "NonNilPointer",
			val:  &i,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, IsNil(tt.val))
		})
	}
}
