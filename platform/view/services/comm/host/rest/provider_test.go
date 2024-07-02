/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertListenAddress(t *testing.T) {
	tests := []struct {
		in  string
		exp string
	}{
		{in: "/ip4/0.0.0.0/tcp/9301", exp: "0.0.0.0:9301"},
	}

	for _, tst := range tests {
		act := convertListenAddress(tst.in)
		assert.Equal(t, tst.exp, act)
	}
}
