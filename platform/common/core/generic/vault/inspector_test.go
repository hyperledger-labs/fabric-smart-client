/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// newTestInspector returns an Inspector over an empty read-write set.
func newTestInspector() *Inspector {
	return &Inspector{Rws: EmptyRWSet()}
}

// TestInspectorReadOnlyPanics pins the read-only contract: every mutating entry
// point panics rather than silently accepting a write. These are guards, not
// behaviour, so the test asserts the panic itself.
func TestInspectorReadOnlyPanics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func(i *Inspector)
	}{
		{"SetState", func(i *Inspector) { _ = i.SetState("ns", "key", []byte("v")) }},
		{"AddReadAt", func(i *Inspector) { _ = i.AddReadAt("ns", "key", nil) }},
		{"DeleteState", func(i *Inspector) { _ = i.DeleteState("ns", "key") }},
		{"SetStateMetadata", func(i *Inspector) { _ = i.SetStateMetadata("ns", "key", nil) }},
		{"SetStateMetadatas", func(i *Inspector) { _ = i.SetStateMetadatas("ns", nil) }},
		{"AppendRWSet", func(i *Inspector) { _ = i.AppendRWSet([]byte("raw")) }},
		{"GetDirectState", func(i *Inspector) { _, _ = i.GetDirectState("ns", "key") }},
		{"Bytes", func(i *Inspector) { _, _ = i.Bytes() }},
		{"Equals", func(i *Inspector) { _ = i.Equals(nil) }},
		{"Clear", func(i *Inspector) { _ = i.Clear("ns") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			i := newTestInspector()
			require.Panics(t, func() { tc.call(i) })
		})
	}
}

// TestInspectorLifecycle covers the two lifecycle methods, which are fixed
// answers: the inspector is never closed and Done is a no-op.
func TestInspectorLifecycle(t *testing.T) {
	t.Parallel()

	i := newTestInspector()

	require.NoError(t, i.IsValid())
	require.False(t, i.IsClosed())

	i.Done()
	require.False(t, i.IsClosed(), "Done must not close the inspector")
}

// TestInspectorGetStateMetadata checks metadata reads resolve against the
// underlying MetaWriteSet, and that a miss returns nil rather than erroring.
func TestInspectorGetStateMetadata(t *testing.T) {
	t.Parallel()

	i := newTestInspector()
	meta := map[string][]byte{"key": []byte("value")}
	require.NoError(t, i.Rws.MetaWriteSet.Add("ns", "k1", meta))

	got, err := i.GetStateMetadata("ns", "k1")
	require.NoError(t, err)
	require.Equal(t, meta, got)

	missing, err := i.GetStateMetadata("ns", "absent")
	require.NoError(t, err)
	require.Nil(t, missing)

	missingNs, err := i.GetStateMetadata("other", "k1")
	require.NoError(t, err)
	require.Nil(t, missingNs)
}

// TestInspectorGetReadKeyAt covers both branches: a position within the
// ordered read set, and one past its end.
func TestInspectorGetReadKeyAt(t *testing.T) {
	t.Parallel()

	i := newTestInspector()
	i.Rws.ReadSet.Add("ns", "k1", nil)
	i.Rws.ReadSet.Add("ns", "k2", nil)

	key, err := i.GetReadKeyAt("ns", 0)
	require.NoError(t, err)
	require.Equal(t, driver.PKey("k1"), key)

	key, err = i.GetReadKeyAt("ns", 1)
	require.NoError(t, err)
	require.Equal(t, driver.PKey("k2"), key)

	_, err = i.GetReadKeyAt("ns", 2)
	require.ErrorContains(t, err, "no read at position 2 for namespace ns")

	_, err = i.GetReadKeyAt("ns", -1)
	require.ErrorContains(t, err, "no read at position -1 for namespace ns")

	_, err = i.GetReadKeyAt("absent", 0)
	require.ErrorContains(t, err, "no read at position 0 for namespace absent")
}

// TestInspectorGetReadAt covers the out-of-range branch alongside the hit,
// which the conformance suite does not reach.
func TestInspectorGetReadAt(t *testing.T) {
	t.Parallel()

	i := newTestInspector()
	i.Rws.ReadSet.Add("ns", "k1", nil)
	require.NoError(t, i.Rws.WriteSet.Add("ns", "k1", []byte("v1")))

	key, val, err := i.GetReadAt("ns", 0)
	require.NoError(t, err)
	require.Equal(t, driver.PKey("k1"), key)
	require.Equal(t, driver.RawValue("v1"), val)

	_, _, err = i.GetReadAt("ns", 1)
	require.ErrorContains(t, err, "no read at position 1 for namespace ns")
}

// TestInspectorGetWriteAt covers the out-of-range branch alongside the hit.
func TestInspectorGetWriteAt(t *testing.T) {
	t.Parallel()

	i := newTestInspector()
	require.NoError(t, i.Rws.WriteSet.Add("ns", "k1", []byte("v1")))

	key, val, err := i.GetWriteAt("ns", 0)
	require.NoError(t, err)
	require.Equal(t, driver.PKey("k1"), key)
	require.Equal(t, driver.RawValue("v1"), val)

	_, _, err = i.GetWriteAt("ns", 1)
	require.ErrorContains(t, err, "no write at position 1 for namespace ns")
}
