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

// TestNamespaceWritesEquals covers the leaf comparison every Equals chain
// bottoms out in: matching sets, differing lengths, a key present in one side
// only, and a value mismatch.
func TestNamespaceWritesEquals(t *testing.T) {
	t.Parallel()

	base := NamespaceWrites{"k1": []byte("v1"), "k2": []byte("v2")}

	require.NoError(t, base.Equals(NamespaceWrites{"k1": []byte("v1"), "k2": []byte("v2")}))

	err := base.Equals(NamespaceWrites{"k1": []byte("v1")})
	require.ErrorContains(t, err, "number of writes do not match")

	err = base.Equals(NamespaceWrites{"k1": []byte("v1"), "other": []byte("v2")})
	require.ErrorContains(t, err, "read not found [k2]")

	err = base.Equals(NamespaceWrites{"k1": []byte("v1"), "k2": []byte("different")})
	require.ErrorContains(t, err, "writes for [k2] do not match")
}

// TestNamespaceWritesKeys checks Keys reports every key held, and nothing for
// an empty set.
func TestNamespaceWritesKeys(t *testing.T) {
	t.Parallel()

	w := NamespaceWrites{"k1": []byte("v1"), "k2": []byte("v2")}
	require.ElementsMatch(t, []string{"k1", "k2"}, w.Keys())

	require.Empty(t, NamespaceWrites{}.Keys())
}

// TestWritesEquals covers the namespace-keyed layer, including the namespace
// filter that restricts comparison to a subset.
func TestWritesEquals(t *testing.T) {
	t.Parallel()

	base := Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("v2")},
	}

	require.NoError(t, base.Equals(Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("v2")},
	}))

	err := base.Equals(Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("changed")},
	})
	require.ErrorContains(t, err, "writes for [ns2] do not match")

	// Restricting to ns1 ignores the ns2 mismatch entirely.
	require.NoError(t, base.Equals(Writes{
		"ns1": NamespaceWrites{"k1": []byte("v1")},
		"ns2": NamespaceWrites{"k2": []byte("changed")},
	}, "ns1"))
}

// TestNamespaceReadsEquals covers the read-side leaf comparison.
func TestNamespaceReadsEquals(t *testing.T) {
	t.Parallel()

	base := NamespaceReads{"k1": Version("v1"), "k2": Version("v2")}

	require.NoError(t, base.Equals(NamespaceReads{"k1": Version("v1"), "k2": Version("v2")}))

	err := base.Equals(NamespaceReads{"k1": Version("v1")})
	require.ErrorContains(t, err, "number of writes do not match")

	err = base.Equals(NamespaceReads{"k1": Version("v1"), "k2": Version("changed")})
	require.ErrorContains(t, err, "writes for [k2] do not match")
}

// TestReadsEquals covers the namespace-keyed read layer and its filter.
func TestReadsEquals(t *testing.T) {
	t.Parallel()

	base := Reads{
		"ns1": NamespaceReads{"k1": Version("v1")},
		"ns2": NamespaceReads{"k2": Version("v2")},
	}

	require.NoError(t, base.Equals(Reads{
		"ns1": NamespaceReads{"k1": Version("v1")},
		"ns2": NamespaceReads{"k2": Version("v2")},
	}))

	err := base.Equals(Reads{
		"ns1": NamespaceReads{"k1": Version("v1")},
		"ns2": NamespaceReads{"k2": Version("changed")},
	})
	require.ErrorContains(t, err, "writes for [ns2] do not match")

	require.NoError(t, base.Equals(Reads{
		"ns1": NamespaceReads{"k1": Version("v1")},
		"ns2": NamespaceReads{"k2": Version("changed")},
	}, "ns1"))
}

// TestKeyedMetaWritesEquals covers metadata comparison, which nests
// entriesEqual one level deeper than the read and write sets.
func TestKeyedMetaWritesEquals(t *testing.T) {
	t.Parallel()

	base := KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("v1")}}

	require.NoError(t, base.Equals(KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("v1")}}))

	err := base.Equals(KeyedMetaWrites{})
	require.ErrorContains(t, err, "number of writes do not match")

	err = base.Equals(KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("changed")}})
	require.ErrorContains(t, err, "writes for [k1] do not match")
}

// TestNamespaceKeyedMetaWritesEquals covers the outermost metadata layer and
// its namespace filter.
func TestNamespaceKeyedMetaWritesEquals(t *testing.T) {
	t.Parallel()

	base := NamespaceKeyedMetaWrites{
		"ns1": KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("v1")}},
		"ns2": KeyedMetaWrites{"k2": MetaWrites{"m2": []byte("v2")}},
	}

	require.NoError(t, base.Equals(NamespaceKeyedMetaWrites{
		"ns1": KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("v1")}},
		"ns2": KeyedMetaWrites{"k2": MetaWrites{"m2": []byte("v2")}},
	}))

	err := base.Equals(NamespaceKeyedMetaWrites{
		"ns1": KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("v1")}},
		"ns2": KeyedMetaWrites{"k2": MetaWrites{"m2": []byte("changed")}},
	})
	require.ErrorContains(t, err, "writes for [ns2] do not match")

	require.NoError(t, base.Equals(NamespaceKeyedMetaWrites{
		"ns1": KeyedMetaWrites{"k1": MetaWrites{"m1": []byte("v1")}},
		"ns2": KeyedMetaWrites{"k2": MetaWrites{"m2": []byte("changed")}},
	}, "ns1"))
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

// TestGetKeysNamespaceFilter covers getKeys directly: unfiltered it returns
// every namespace, filtered it returns the intersection.
func TestGetKeysNamespaceFilter(t *testing.T) {
	t.Parallel()

	m := map[driver.Namespace]NamespaceWrites{
		"ns1": {"k1": []byte("v1")},
		"ns2": {"k2": []byte("v2")},
	}

	require.ElementsMatch(t, []string{"ns1", "ns2"}, getKeys(m))
	require.ElementsMatch(t, []string{"ns1"}, getKeys(m, "ns1"))
	require.Empty(t, getKeys(m, "absent"))
}

// TestAddRejectsInvalidNamespace covers the validation branch both Add
// implementations share, which returns before touching the underlying map.
func TestAddRejectsInvalidNamespace(t *testing.T) {
	t.Parallel()

	rws := EmptyRWSet()

	err := rws.WriteSet.Add("", "k1", []byte("v1"))
	require.Error(t, err)
	require.Empty(t, rws.Writes, "a rejected write must not create the namespace")

	err = rws.MetaWriteSet.Add("", "k1", map[string][]byte{"m1": []byte("v1")})
	require.Error(t, err)
	require.Empty(t, rws.MetaWrites, "a rejected write must not create the namespace")
}
