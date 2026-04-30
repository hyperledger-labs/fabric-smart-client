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

func TestNoErrorDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		NoError(nil)
	})
}

func TestNoErrorPanicsAndReleases(t *testing.T) {
	t.Parallel()

	released := false
	require.Panics(t, func() {
		NoError(errors.New("boom"), func() { released = true })
	})
	require.True(t, released)
}

func TestValidIdentityReturnsIdentity(t *testing.T) {
	t.Parallel()

	id := view.Identity("alice")
	require.Equal(t, id, ValidIdentity(id, nil))
}

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

func TestExtractReleasersSeparatesFunctions(t *testing.T) {
	t.Parallel()

	releaser := func() {}
	msgs, releasers := extractReleasers("hello", 42, releaser)
	require.Equal(t, []interface{}{"hello", 42}, msgs)
	require.Len(t, releasers, 1)
}
