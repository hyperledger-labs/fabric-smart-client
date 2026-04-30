/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package assert

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestValidIdentityPanicsWhenIdentityIsNil(t *testing.T) {
	t.Parallel()

	released := false

	require.Panics(t, func() {
		ValidIdentity(nil, nil, func() { released = true })
	})
	require.True(t, released)
}

func TestValidIdentityPanicsWhenErrorIsNotNil(t *testing.T) {
	t.Parallel()

	released := false

	require.Panics(t, func() {
		ValidIdentity(view.Identity([]byte("alice")), errors.New("boom"), func() { released = true })
	})
	require.True(t, released)
}
