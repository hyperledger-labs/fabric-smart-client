/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLogSpec checks CORE_LOGGING_SPEC wins over FABRIC_LOGGING_SPEC, and an empty variable
// counts as unset.
func TestLogSpec(t *testing.T) { //nolint:paralleltest // uses t.Setenv
	for _, tc := range []struct{ name, core, fabric, want string }{
		{"default", "", "", "grpc=error:libp2p=warn:info"},
		{"fabric", "", "warn", "warn"},
		{"core wins", "debug", "warn", "debug"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CORE_LOGGING_SPEC", tc.core)
			t.Setenv("FABRIC_LOGGING_SPEC", tc.fabric)
			assert.Equal(t, tc.want, logSpec())
		})
	}
}
