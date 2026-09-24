/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Filter yields an empty stream when nothing matches, so At is reachable with an
// unchecked position.
func TestStreamAtOutOfRange(t *testing.T) {
	t.Parallel()

	os := &outputStream{outputs: []*output{{key: ID("k")}}}
	require.NotNil(t, os.At(0))
	require.Nil(t, os.At(1))
	require.Nil(t, os.At(-1))

	is := &inputStream{inputs: []*input{{key: ID("k")}}}
	require.NotNil(t, is.At(0))
	require.Nil(t, is.At(1))
	require.Nil(t, is.At(-1))

	cs := &commandStream{commands: []*Command{{Name: "cmd"}}}
	require.NotNil(t, cs.At(0))
	require.Nil(t, cs.At(1))
	require.Nil(t, cs.At(-1))

	// An empty stream, as produced by a Filter that matches nothing.
	require.Nil(t, os.Filter(func(*output) bool { return false }).At(0))
	require.Nil(t, is.Filter(func(*input) bool { return false }).At(0))
	require.Nil(t, cs.Filter(func(*Command) bool { return false }).At(0))
}

// Callers chain off At's result, so the error-returning methods report a nil
// receiver instead of dereferencing it.
func TestNilElementMethodsReturnError(t *testing.T) {
	t.Parallel()

	var o *output
	require.ErrorContains(t, o.State(nil), "output not found")

	var i *input
	require.ErrorContains(t, i.State(nil), "input not found")
	require.ErrorContains(t, i.VerifyCertification(), "input not found")
}

// ID and IsDelete have no error to carry. A zero ID would hide the mistake at
// the call site, so the panic stands and At documents the requirement.
func TestNilElementValueMethodsPanic(t *testing.T) {
	t.Parallel()

	var o *output
	require.Panics(t, func() { _ = o.ID() })
	require.Panics(t, func() { _ = o.IsDelete() })

	var i *input
	require.Panics(t, func() { _ = i.ID() })
}
