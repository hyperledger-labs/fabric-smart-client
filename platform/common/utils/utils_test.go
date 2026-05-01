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

type mockCloser struct {
	called bool
	err    error
}

func (m *mockCloser) Close() error {
	m.called = true
	return m.err
}

func TestCloseMute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		closer     interface{}
		wantCalled bool
	}{
		{
			name:       "NilCloser",
			closer:     nil,
			wantCalled: false,
		},
		{
			name:       "ValidCloser",
			closer:     &mockCloser{},
			wantCalled: true,
		},
		{
			name:       "CloserWithError",
			closer:     &mockCloser{err: errors.New("close error")},
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var mc *mockCloser
			if tt.closer != nil {
				mc = tt.closer.(*mockCloser)
				CloseMute(mc)
				require.Equal(t, tt.wantCalled, mc.called)
			} else {
				require.NotPanics(t, func() { CloseMute(nil) })
			}
		})
	}
}

func TestIgnoreError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "NilError",
			err:  nil,
		},
		{
			name: "SomeError",
			err:  errors.New("some error"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() { IgnoreError(tt.err) })
		})
	}
}

func TestIgnoreErrorFunc(t *testing.T) {
	t.Parallel()
	called := false
	IgnoreErrorFunc(func() error {
		called = true
		return errors.New("func error")
	})
	require.True(t, called)
}

func TestIgnoreErrorWithOneArg(t *testing.T) {
	t.Parallel()
	calledWith := ""
	IgnoreErrorWithOneArg(func(s string) error {
		calledWith = s
		return errors.New("arg error")
	}, "hello")
	require.Equal(t, "hello", calledWith)
}

func TestZero(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, Zero[int]())
	require.Equal(t, "", Zero[string]())
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
		tt := tt
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
		tt := tt
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

func TestDefaultZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  interface{}
		want int
	}{
		{
			name: "NilValue",
			val:  nil,
			want: 0,
		},
		{
			name: "IntValue",
			val:  10,
			want: 10,
		},
		{
			name: "NonIntValue",
			val:  "not int",
			want: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, DefaultZero[int](tt.val))
		})
	}
}

func TestDefaultInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  interface{}
		def  int
		want int
	}{
		{
			name: "NilValue",
			val:  nil,
			def:  5,
			want: 5,
		},
		{
			name: "IntValue",
			val:  10,
			def:  5,
			want: 10,
		},
		{
			name: "ZeroValue",
			val:  0,
			def:  5,
			want: 5,
		},
		{
			name: "NonIntValue",
			val:  "not int",
			def:  5,
			want: 5,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, DefaultInt(tt.val, tt.def))
		})
	}
}

func TestDefaultString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  interface{}
		def  string
		want string
	}{
		{
			name: "NilValue",
			val:  nil,
			def:  "default",
			want: "default",
		},
		{
			name: "StringValue",
			val:  "value",
			def:  "default",
			want: "value",
		},
		{
			name: "EmptyString",
			val:  "",
			def:  "default",
			want: "default",
		},
		{
			name: "NonStringValue",
			val:  10,
			def:  "default",
			want: "default",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, DefaultString(tt.val, tt.def))
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
		val  interface{}
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, IsNil(tt.val))
		})
	}
}
