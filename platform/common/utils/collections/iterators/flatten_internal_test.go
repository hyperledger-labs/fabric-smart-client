/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenIsNil(t *testing.T) {
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
			require.Equal(t, tt.want, isNil(tt.val))
		})
	}
}
