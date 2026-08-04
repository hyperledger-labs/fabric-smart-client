/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// stringHashState has a string field tagged for hash-hiding, which is unsupported:
// marshalTags cannot retain a recoverable/verifiable preimage for it, so both the
// marshal and unmarshal paths must fail closed rather than emit or accept an
// unverifiable hash.
type stringHashState struct {
	ID     string `json:"id"`
	Secret string `state:"hash" json:"secret"`
}

func TestMarshalTagsRejectsStringHashField(t *testing.T) {
	t.Parallel()

	_, _, err := (&Namespace{}).marshalTags(nil, &stringHashState{ID: "1", Secret: "top-secret"})
	require.Error(t, err)
	require.ErrorContains(t, err, "Secret")
	require.ErrorContains(t, err, "not supported")
}

func TestUnmarshalTagsRejectsStringHashField(t *testing.T) {
	t.Parallel()

	err := (&Namespace{}).unmarshalTags(nil, &stringHashState{ID: "1", Secret: "top-secret"}, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "Secret")
	require.ErrorContains(t, err, "not supported")
}
