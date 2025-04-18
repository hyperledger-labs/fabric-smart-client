/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input          []string
	expectedOutput string
}

func TestEscapeTableName(t *testing.T) {
	cases := []testCase{
		{[]string{}, ""},
		{[]string{"alpha", "testchannel"}, "alpha__testchannel"},
		{[]string{"alpha", "test-channel"}, "alpha__test_dchannel"},
		{[]string{"alpha", "test-channel", "other.param"}, "alpha__test_dchannel__other_fparam"},
	}
	f, err := newTableNameFormatter("")
	assert.NoError(t, err)
	for _, c := range cases {
		assert.Equal(t, c.expectedOutput, f.escapeForTableName(c.input...))
	}
}

func TestEscapeTableNameError(t *testing.T) {
	cases := [][]string{
		{"alpha", "testchannel!"},
		{"alpha", "test-#channel"},
	}
	f, err := newTableNameFormatter("")
	assert.NoError(t, err)
	for _, c := range cases {
		assert.Panics(t, func() { f.escapeForTableName(c...) })
	}
}
