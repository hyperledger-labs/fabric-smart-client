/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEntriesEqual drives every branch of the comparison all six Equals
// implementations funnel through — length mismatch, missing key, value
// mismatch, match — plus the namespace filter in getKeys.
func TestEntriesEqual(t *testing.T) {
	t.Parallel()

	base := Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("v2")},
	}
	same := Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("v2")},
	}
	changed := Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("changed")},
	}

	require.NoError(t, base.Equals(same))
	require.ErrorContains(t, base.Equals(Writes{"ns1": base["ns1"]}), "number of entries do not match [2]!=[1]")
	require.ErrorContains(t, base.Equals(Writes{"ns1": base["ns1"], "other": nil}), "key not found [ns2]")
	require.ErrorContains(t, base.Equals(changed), "entries for [ns2] do not match")

	require.NoError(t, base.Equals(changed, "ns1"), "the filter must ignore the ns2 mismatch")
	require.ErrorContains(t, base.Equals(Writes{"ns1": base["ns1"]}, "ns1", "ns2"),
		"number of entries do not match [2]!=[1]", "the reported counts are the filtered ones")
	require.NoError(t, base.Equals(same, "absent"), "a filter matching no namespace compares nothing")
}

// TestEqualsWrappers checks each remaining Equals delegates to entriesEqual,
// matching and mismatching. KeyedMetaWrites nests it one level deeper.
func TestEqualsWrappers(t *testing.T) {
	t.Parallel()

	require.NoError(t, NamespaceWrites{"k": []byte("v")}.Equals(NamespaceWrites{"k": []byte("v")}))
	require.Error(t, NamespaceWrites{"k": []byte("v")}.Equals(NamespaceWrites{"k": []byte("x")}))

	require.NoError(t, NamespaceReads{"k": Version("v")}.Equals(NamespaceReads{"k": Version("v")}))
	require.Error(t, NamespaceReads{"k": Version("v")}.Equals(NamespaceReads{"k": Version("x")}))

	reads := Reads{"ns": NamespaceReads{"k": Version("v")}}
	require.NoError(t, reads.Equals(Reads{"ns": NamespaceReads{"k": Version("v")}}))
	require.Error(t, reads.Equals(Reads{"ns": NamespaceReads{"k": Version("x")}}))

	meta := KeyedMetaWrites{"k": MetaWrites{"m": []byte("v")}}
	require.NoError(t, meta.Equals(KeyedMetaWrites{"k": MetaWrites{"m": []byte("v")}}))
	require.Error(t, meta.Equals(KeyedMetaWrites{"k": MetaWrites{"m": []byte("x")}}))

	nsMeta := NamespaceKeyedMetaWrites{"ns": meta}
	require.NoError(t, nsMeta.Equals(NamespaceKeyedMetaWrites{"ns": meta}))
	require.Error(t, nsMeta.Equals(NamespaceKeyedMetaWrites{
		"ns": KeyedMetaWrites{"k": MetaWrites{"m": []byte("x")}},
	}))
}

// TestNamespaceWritesKeys checks Keys reports every key held, and nothing for
// an empty set.
func TestNamespaceWritesKeys(t *testing.T) {
	t.Parallel()

	require.ElementsMatch(t, []string{"k1", "k2"}, NamespaceWrites{"k1": []byte("v1"), "k2": []byte("v2")}.Keys())
	require.Empty(t, NamespaceWrites{}.Keys())
}

// TestWriteSetClear checks Clear empties one namespace and leaves others
// intact, including its ordered-key bookkeeping.
func TestWriteSetClear(t *testing.T) {
	t.Parallel()

	rws := EmptyRWSet()
	require.NoError(t, rws.WriteSet.Add("ns1", "k1", []byte("v1")))
	require.NoError(t, rws.WriteSet.Add("ns2", "k2", []byte("v2")))

	rws.WriteSet.Clear("ns1")

	require.False(t, rws.WriteSet.In("ns1", "k1"))
	require.Empty(t, rws.Writes["ns1"])
	require.Empty(t, rws.OrderedWrites["ns1"])

	_, in := rws.WriteSet.GetAt("ns1", 0)
	require.False(t, in, "cleared namespace must have no ordered writes")

	require.True(t, rws.WriteSet.In("ns2", "k2"), "other namespaces are untouched")
}

// TestReadSetClear checks the read-side equivalent.
func TestReadSetClear(t *testing.T) {
	t.Parallel()

	rws := EmptyRWSet()
	rws.ReadSet.Add("ns1", "k1", Version("v1"))
	rws.ReadSet.Add("ns2", "k2", Version("v2"))

	rws.ReadSet.Clear("ns1")

	_, in := rws.ReadSet.Get("ns1", "k1")
	require.False(t, in)
	require.Empty(t, rws.OrderedReads["ns1"])

	_, in = rws.ReadSet.GetAt("ns1", 0)
	require.False(t, in, "cleared namespace must have no ordered reads")

	_, in = rws.ReadSet.Get("ns2", "k2")
	require.True(t, in, "other namespaces are untouched")
}

// TestMetaWriteSetClear checks the metadata-side equivalent.
func TestMetaWriteSetClear(t *testing.T) {
	t.Parallel()

	rws := EmptyRWSet()
	require.NoError(t, rws.MetaWriteSet.Add("ns1", "k1", map[string][]byte{"m1": []byte("v1")}))
	require.NoError(t, rws.MetaWriteSet.Add("ns2", "k2", map[string][]byte{"m2": []byte("v2")}))

	rws.MetaWriteSet.Clear("ns1")

	require.False(t, rws.MetaWriteSet.In("ns1", "k1"))
	require.Empty(t, rws.MetaWriteSet.Get("ns1", "k1"))

	require.True(t, rws.MetaWriteSet.In("ns2", "k2"), "other namespaces are untouched")
}

// TestAddRejectsInvalidNamespace covers the validation branch both Add
// implementations share, which returns before touching the underlying map.
func TestAddRejectsInvalidNamespace(t *testing.T) {
	t.Parallel()

	rws := EmptyRWSet()

	require.Error(t, rws.WriteSet.Add("", "k1", []byte("v1")))
	require.Empty(t, rws.Writes, "a rejected write must not create the namespace")

	require.Error(t, rws.MetaWriteSet.Add("", "k1", map[string][]byte{"m1": []byte("v1")}))
	require.Empty(t, rws.MetaWrites, "a rejected write must not create the namespace")
}

// TestSetAddIsIdempotent pins the ordered-key bookkeeping when the same key is
// recorded twice. The ordered slice must stay in step with the map: callers
// iterate 0..NumReads-1 and index the slice (KeyExist in platform/fabric/vault.go,
// anyKeyContains in platform/fabricx/core/transaction/rwset/loader.go), so a
// duplicate entry would hide every key recorded after it.
func TestSetAddIsIdempotent(t *testing.T) {
	t.Parallel()

	rws := EmptyRWSet()
	rws.ReadSet.Add("ns", "k1", Version("v1"))
	rws.ReadSet.Add("ns", "k1", Version("v2"))
	rws.ReadSet.Add("ns", "k2", Version("v3"))

	require.Equal(t, []string{"k1", "k2"}, rws.OrderedReads["ns"])
	require.Len(t, rws.Reads["ns"], len(rws.OrderedReads["ns"]))

	version, in := rws.ReadSet.Get("ns", "k1")
	require.True(t, in)
	require.Equal(t, Version("v2"), version, "the later read wins")

	// WriteSet.Add already behaved this way; assert it so the two stay aligned.
	require.NoError(t, rws.WriteSet.Add("ns", "k1", []byte("v1")))
	require.NoError(t, rws.WriteSet.Add("ns", "k1", []byte("v2")))
	require.NoError(t, rws.WriteSet.Add("ns", "k2", []byte("v3")))

	require.Equal(t, []string{"k1", "k2"}, rws.OrderedWrites["ns"])
	require.Equal(t, []byte("v2"), rws.WriteSet.Get("ns", "k1"), "the later write wins")
}
