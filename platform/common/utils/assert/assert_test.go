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

	assertPanicsAndReleases(t, func(release func()) {
		NoError(errors.New("boom"), release)
	})
}

func TestErrorDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		Error(errors.New("boom"))
	})
}

func TestErrorPanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		Error(nil, release)
	})
}

func TestNotNilDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		NotNil("value")
	})
}

func TestNotNilPanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		NotNil(nil, release)
	})
}

func TestNotEmptyDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		NotEmpty("value")
	})
}

func TestNotEmptyPanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		NotEmpty("", release)
	})
}

func TestEqualDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		Equal("alice", "alice")
	})
}

func TestEqualPanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		Equal("alice", "bob", release)
	})
}

func TestNotEqualDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		NotEqual("alice", "bob")
	})
}

func TestNotEqualPanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		NotEqual("alice", "alice", release)
	})
}

func TestTrueDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		True(true)
	})
}

func TestTruePanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		True(false, release)
	})
}

func TestFalseDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		False(false)
	})
}

func TestFalsePanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		False(true, release)
	})
}

func TestFailPanicsAndReleases(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		Fail("boom", release)
	})
}

func TestValidIdentityReturnsIdentity(t *testing.T) {
	t.Parallel()

	id := view.Identity("alice")
	require.Equal(t, id, ValidIdentity(id, nil))
}

func TestValidIdentityPanicsWhenIdentityIsNil(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		ValidIdentity(nil, nil, release)
	})
}

func TestValidIdentityPanicsWhenErrorIsNotNil(t *testing.T) {
	t.Parallel()

	assertPanicsAndReleases(t, func(release func()) {
		ValidIdentity(view.Identity([]byte("alice")), errors.New("boom"), release)
	})
}

func TestExtractReleasersSeparatesFunctions(t *testing.T) {
	t.Parallel()

	releaser := func() {}
	msgs, releasers := extractReleasers("hello", 42, releaser)
	require.Equal(t, []interface{}{"hello", 42}, msgs)
	require.Len(t, releasers, 1)
}

func assertPanicsAndReleases(t *testing.T, f func(release func())) {
	t.Helper()

	released := false
	require.Panics(t, func() {
		f(func() { released = true })
	})
	require.True(t, released)
}
