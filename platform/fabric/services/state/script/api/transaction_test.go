/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputEquals(t *testing.T) {
	t.Parallel()

	newOutput := func() *Output {
		return &Output{
			Reference: &StateReference{TxID: "tx", Index: 1},
			ID:        "id",
			Raw:       []byte("raw"),
			Birth:     Script{Type: "birth", Raw: []byte("b")},
			Death:     Script{Type: "death", Raw: []byte("d")},
		}
	}

	tests := []struct {
		name   string
		modify func(o *Output) *Output
		equal  bool
	}{
		{name: "same fields", modify: func(o *Output) *Output { return o }, equal: true},
		{name: "nil other", modify: func(*Output) *Output { return nil }, equal: false},
		{name: "different reference", modify: func(o *Output) *Output { o.Reference = &StateReference{TxID: "tx", Index: 2}; return o }, equal: false},
		{name: "nil reference", modify: func(o *Output) *Output { o.Reference = nil; return o }, equal: false},
		{name: "different ID", modify: func(o *Output) *Output { o.ID = "other"; return o }, equal: false},
		{name: "different raw", modify: func(o *Output) *Output { o.Raw = []byte("other"); return o }, equal: false},
		{name: "different birth", modify: func(o *Output) *Output { o.Birth.Raw = []byte("other"); return o }, equal: false},
		{name: "different death", modify: func(o *Output) *Output { o.Death.Type = "other"; return o }, equal: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.equal, newOutput().Equals(tt.modify(newOutput())))
		})
	}

	t.Run("both references nil", func(t *testing.T) {
		t.Parallel()
		o, o2 := newOutput(), newOutput()
		o.Reference, o2.Reference = nil, nil
		require.True(t, o.Equals(o2))
	})
}
