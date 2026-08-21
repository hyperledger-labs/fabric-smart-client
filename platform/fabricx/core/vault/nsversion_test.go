/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

// nsVersionOf decodes serialized RWSet bytes and returns the NsVersion carried for ns.
func nsVersionOf(t *testing.T, raw []byte, ns driver.Namespace) uint64 {
	t.Helper()
	var tx applicationpb.Tx
	require.NoError(t, proto.Unmarshal(raw, &tx))
	for _, txNs := range tx.GetNamespaces() {
		if txNs.GetNsId() == string(ns) {
			return txNs.GetNsVersion()
		}
	}
	t.Fatalf("namespace %s not present in serialized rwset", ns)
	return 0
}

// Bytes() is a pure function of RWSet state: the namespace versions are resolved when a
// namespace is first touched, so serialization itself must never reach the query service.
func TestRWSet_Bytes_IssuesNoQueries(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))

	statesBefore := qs.getStatesCount.Load()

	first, err := rws.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, first)

	for range 4 {
		again, err := rws.Bytes()
		require.NoError(t, err)
		require.Equal(t, first, again, "Bytes() must be deterministic across calls")
	}

	require.Equal(t, statesBefore, qs.getStatesCount.Load(), "Bytes() must not query state")
}

// The namespace version belongs to the simulation snapshot, alongside the key read
// versions captured by GetState. A _meta bump after the namespace was touched must not
// change what this RWSet serializes to.
func TestRWSet_NamespaceVersionPinnedAtFirstTouch(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))

	// The namespace policy is updated after the namespace entered the RWSet.
	qs.setState("_meta", "ns1", nil, 9)

	raw, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(7), nsVersionOf(t, raw, "ns1"))
}

// A namespace with no _meta entry serializes with version 0, matching the "unknown
// namespace" fallback.
func TestRWSet_UnknownNamespaceSerializesVersionZero(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))

	raw, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(0), nsVersionOf(t, raw, "ns1"))
}

// Namespace versions are resolved per RWSet, exactly like the key read versions GetState
// captures: nothing is cached across transactions, so each RWSet sees the version current
// when it touched the namespace, and a later transaction picks up an updated one.
func TestVault_NamespaceVersionResolvedPerRWSet(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)
	ctx := context.Background()

	first, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NoError(t, first.SetState("ns1", "key1", []byte("val1")))

	// A namespace policy update lands between the two transactions.
	qs.setState("_meta", "ns1", nil, 9)

	second, err := v.NewRWSet(ctx, "tx2")
	require.NoError(t, err)
	require.NoError(t, second.SetState("ns1", "key1", []byte("val1")))

	firstRaw, err := first.Bytes()
	require.NoError(t, err)
	secondRaw, err := second.Bytes()
	require.NoError(t, err)

	require.Equal(t, uint64(7), nsVersionOf(t, firstRaw, "ns1"),
		"the in-flight RWSet keeps the version it simulated against")
	require.Equal(t, uint64(9), nsVersionOf(t, secondRaw, "ns1"),
		"a new RWSet must pick up the updated version without a restart")
}

// Touching the same namespace repeatedly resolves its version once; only the first touch
// reaches the query service.
func TestRWSet_NamespaceVersionResolvedOncePerNamespace(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)

	before := qs.getStatesCount.Load()
	for i := range 5 {
		require.NoError(t, rws.SetState("ns1", driver.PKey(string(rune('a'+i))), []byte("val")))
	}
	require.Equal(t, before+1, qs.getStatesCount.Load(),
		"only the first touch of a namespace resolves its version")
}

// An endorser reconstructs the RWSet from the proposer's bytes and re-serializes it to
// sign. The signature covers NsVersion, so re-serialization must reproduce the version
// the proposer used, not whatever _meta says at endorsement time.
func TestRWSet_FromBytesPreservesNamespaceVersion(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)
	ctx := context.Background()

	proposer, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NoError(t, proposer.SetState("ns1", "key1", []byte("val1")))
	proposed, err := proposer.Bytes()
	require.NoError(t, err)

	// The namespace policy moves on before the endorser reconstructs the transaction.
	qs.setState("_meta", "ns1", nil, 9)

	endorser, err := v.NewRWSetFromBytes(ctx, "tx1", proposed)
	require.NoError(t, err)
	reserialized, err := endorser.Bytes()
	require.NoError(t, err)

	require.Equal(t, uint64(7), nsVersionOf(t, reserialized, "ns1"))
	require.Equal(t, proposed, reserialized,
		"an endorser must sign over the same bytes the proposer produced")
}

// AppendRWSet carries namespace versions in the appended payload the same way
// NewRWSetFromBytes does.
func TestRWSet_AppendPreservesNamespaceVersion(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)
	ctx := context.Background()

	source, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NoError(t, source.SetState("ns1", "key1", []byte("val1")))
	raw, err := source.Bytes()
	require.NoError(t, err)

	qs.setState("_meta", "ns1", nil, 9)

	target, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NoError(t, target.AppendRWSet(raw))

	appended, err := target.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(7), nsVersionOf(t, appended, "ns1"))
}

// First pin wins: a namespace already touched locally keeps the version it resolved, and
// an appended payload does not overwrite it. Documented so the ordering hazard is explicit
// — appending into an RWSet that already touched the namespace does not reproduce the
// proposer's bytes.
func TestRWSet_AppendDoesNotOverwriteAnEarlierPin(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)
	ctx := context.Background()

	source, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NoError(t, source.SetState("ns1", "key1", []byte("val1")))
	raw, err := source.Bytes()
	require.NoError(t, err)

	qs.setState("_meta", "ns1", nil, 9)

	target, err := v.NewRWSet(ctx, "tx2")
	require.NoError(t, err)
	// The namespace is pinned at 9 here, before the payload carrying version 7 arrives.
	require.NoError(t, target.SetState("ns1", "key2", []byte("val2")))
	require.NoError(t, target.AppendRWSet(raw))

	appended, err := target.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(9), nsVersionOf(t, appended, "ns1"))
}

// Resolving the namespace version happens on first touch, so that is where a query
// service failure must surface — not later, from serialization.
func TestRWSet_FirstTouchReportsVersionLookupError(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.getStatesErr = errors.New("simulated lookup failure")
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)

	err = rws.SetState("ns1", "key1", []byte("val1"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "simulated lookup failure")
}

// GetState is a namespace touch too: reading pins the version alongside the key read
// version it adds to the read set.
func TestRWSet_GetStatePinsNamespaceVersion(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	qs.setState("ns1", "key1", []byte("val1"), 3)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)

	val, err := rws.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, []byte("val1"), val)

	qs.setState("_meta", "ns1", nil, 9)

	raw, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(7), nsVersionOf(t, raw, "ns1"))
}

// Concurrent first touches of the same namespace must agree on one pinned version;
// otherwise two Bytes() calls on the same RWSet could disagree.
func TestRWSet_ConcurrentFirstTouchPinsSingleVersion(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)

	var eg errgroup.Group
	for g := range 16 {
		eg.Go(func() error {
			return rws.SetState("ns1", driver.PKey(string(rune('a'+g))), []byte("value"))
		})
	}
	require.NoError(t, eg.Wait())

	raw, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(7), nsVersionOf(t, raw, "ns1"))
}

// Bytes() passes the pinned versions straight to Marshal, which rejects a namespace it has
// no version for. Every mutating entry point must therefore pin, or serialization fails at
// runtime. This walks each of them.
func TestRWSet_EveryMutatorPinsNamespace(t *testing.T) {
	t.Parallel()

	mutators := map[string]func(t *testing.T, rws driver.RWSet){
		"SetState": func(t *testing.T, rws driver.RWSet) {
			t.Helper()
			require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
		},
		"DeleteState": func(t *testing.T, rws driver.RWSet) {
			t.Helper()
			require.NoError(t, rws.DeleteState("ns1", "key1"))
		},
		"AddReadAt": func(t *testing.T, rws driver.RWSet) {
			t.Helper()
			require.NoError(t, rws.AddReadAt("ns1", "key1", vault.MarshalVersion(3)))
		},
		"GetState": func(t *testing.T, rws driver.RWSet) {
			t.Helper()
			_, err := rws.GetState("ns1", "key1")
			require.NoError(t, err)
		},
		"SetStateMetadata": func(t *testing.T, rws driver.RWSet) {
			t.Helper()
			require.NoError(t, rws.SetStateMetadata("ns1", "key1", driver.Metadata{"m": []byte("v")}))
		},
	}

	for name, mutate := range mutators {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			qs := newMockQueryService()
			qs.setState("_meta", "ns1", nil, 7)
			qs.setState("ns1", "key1", []byte("val1"), 3)
			v := vault.NewVault(qs, nil)

			rws, err := v.NewRWSet(context.Background(), "tx1")
			require.NoError(t, err)
			mutate(t, rws)

			// Serialization must succeed; an unpinned namespace makes Marshal fail with
			// "nsInfo does not contain entry for ns".
			raw, err := rws.Bytes()
			require.NoError(t, err)

			// SetStateMetadata alone produces no namespace in the FabricX encoding, which
			// only carries reads and writes; the rest must carry the pinned version.
			if name != "SetStateMetadata" {
				require.Equal(t, uint64(7), nsVersionOf(t, raw, "ns1"))
			}
		})
	}
}

// A namespace re-touched after Clear is re-pinned rather than serialized without a version.
func TestRWSet_ClearedNamespaceIsRepinned(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
	require.NoError(t, rws.Clear("ns1"))

	qs.setState("_meta", "ns1", nil, 9)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))

	raw, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, uint64(9), nsVersionOf(t, raw, "ns1"),
		"Clear drops the pin, so the next touch resolves a fresh version")
}

// IsValid checks the whole simulation snapshot, and the pinned namespace version is part
// of it: the committer validates NsVersion as a read of _meta[ns], so a namespace policy
// update invalidates the transaction just like a conflicting key read does.
func TestRWSet_IsValid_DetectsNamespaceVersionChange(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
	require.NoError(t, rws.IsValid())

	qs.setState("_meta", "ns1", nil, 9)

	err = rws.IsValid()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ns1")
}

// A namespace that gets registered after it was touched as unknown (version 0) also
// invalidates the snapshot.
func TestRWSet_IsValid_DetectsNamespaceRegisteredAfterTouch(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
	require.NoError(t, rws.IsValid())

	qs.setState("_meta", "ns1", nil, 1)

	require.Error(t, rws.IsValid())
}

// Validating reads and namespace versions is a single batched round-trip, not one call
// per key.
func TestRWSet_IsValid_BatchesLookups(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	for _, k := range []driver.PKey{"key1", "key2", "key3"} {
		qs.setState("ns1", k, []byte("val"), 1)
	}
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	for _, k := range []driver.PKey{"key1", "key2", "key3"} {
		_, err := rws.GetState("ns1", k)
		require.NoError(t, err)
	}

	before := qs.getStatesCount.Load()
	require.NoError(t, rws.IsValid())
	require.Equal(t, before+1, qs.getStatesCount.Load(),
		"IsValid should issue one batched query, not one per read")
}

// The _meta key list is built from both the RWSet's own reads and the pinned namespaces.
// Those two sources overlap when the RWSet reads inside _meta, and the batch must not
// carry the same key twice.
func TestRWSet_IsValid_DoesNotDuplicateMetaKeys(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
	// Read a key that lives in _meta, so the read keys and the pinned namespaces overlap.
	_, err = rws.GetState("_meta", "ns1")
	require.NoError(t, err)

	require.NoError(t, rws.IsValid())

	metaKeys := qs.lastQuery()["_meta"]
	seen := make(map[driver.PKey]int, len(metaKeys))
	for _, k := range metaKeys {
		seen[k]++
	}
	for k, n := range seen {
		require.Equal(t, 1, n, "key %s appears %d times in the _meta batch", k, n)
	}
}

// Append's namespace filter is documented to restrict what is deserialized; InspectRWSet
// and AppendRWSet both forward a caller-supplied list to it.
func TestRWSet_AppendHonoursNamespaceFilter(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	qs.setState("_meta", "ns2", nil, 8)
	v := vault.NewVault(qs, nil)
	ctx := context.Background()

	source, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NoError(t, source.SetState("ns1", "key1", []byte("val1")))
	require.NoError(t, source.SetState("ns2", "key2", []byte("val2")))
	raw, err := source.Bytes()
	require.NoError(t, err)

	t.Run("AppendRWSet", func(t *testing.T) {
		t.Parallel()
		target, err := v.NewRWSet(ctx, "tx2")
		require.NoError(t, err)
		require.NoError(t, target.AppendRWSet(raw, "ns1"))

		require.Equal(t, 1, target.NumWrites("ns1"))
		require.Equal(t, 0, target.NumWrites("ns2"), "ns2 must be filtered out")
		require.Equal(t, []driver.Namespace{"ns1"}, target.Namespaces())
	})

	t.Run("InspectRWSet", func(t *testing.T) {
		t.Parallel()
		inspected, err := v.InspectRWSet(ctx, raw, "ns2")
		require.NoError(t, err)

		require.Equal(t, 1, inspected.NumWrites("ns2"))
		require.Equal(t, 0, inspected.NumWrites("ns1"), "ns1 must be filtered out")
		require.Equal(t, []driver.Namespace{"ns2"}, inspected.Namespaces())

		// The filtered result must still serialize, which requires the pinned version for
		// the namespace that survived the filter.
		out, err := inspected.Bytes()
		require.NoError(t, err)
		require.Equal(t, uint64(8), nsVersionOf(t, out, "ns2"))
	})

	t.Run("NoFilterKeepsEverything", func(t *testing.T) {
		t.Parallel()
		target, err := v.NewRWSet(ctx, "tx3")
		require.NoError(t, err)
		require.NoError(t, target.AppendRWSet(raw))
		require.Equal(t, 1, target.NumWrites("ns1"))
		require.Equal(t, 1, target.NumWrites("ns2"))
	})
}

// namespacesOf decodes serialized RWSet bytes and returns the namespace ids it carries.
func namespacesOf(t *testing.T, raw []byte) []string {
	t.Helper()
	var tx applicationpb.Tx
	require.NoError(t, proto.Unmarshal(raw, &tx))
	nss := make([]string, 0, len(tx.GetNamespaces()))
	for _, txNs := range tx.GetNamespaces() {
		nss = append(nss, txNs.GetNsId())
	}
	return nss
}

// Clear drops the namespace's pinned version, so it must drop the namespace from the
// RWSet as well: a namespace that survives in the read/write maps without a pin makes
// Marshal reject the whole RWSet.
func TestRWSet_ClearedNamespaceStillSerializes(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("_meta", "ns1", nil, 7)
	qs.setState("_meta", "ns2", nil, 8)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
	require.NoError(t, rws.SetState("ns2", "key2", []byte("val2")))

	require.NoError(t, rws.Clear("ns1"))

	raw, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, []string{"ns2"}, namespacesOf(t, raw),
		"a cleared namespace must not survive into the serialized form")
	require.NotContains(t, rws.Namespaces(), driver.Namespace("ns1"))
}

// A read of a key that does not exist is serialized with no version at all. Append must
// preserve that, because an endorser re-serializes what it received and signs the result.
func TestRWSet_VersionlessReadRoundTrips(t *testing.T) {
	t.Parallel()
	first, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{
		NsId:      "ns1",
		NsVersion: 7,
		ReadsOnly: []*applicationpb.Read{{Key: []byte("missing-key"), Version: nil}},
	}}})
	require.NoError(t, err)

	m := vault.NewMarshaller()
	rws, nsVersions, err := m.RWSetFromBytes(first)
	require.NoError(t, err)
	second, err := m.Marshal("tx1", rws, nsVersions)
	require.NoError(t, err)

	require.Equal(t, first, second, "a versionless read must re-serialize unchanged")
}

// A ReadWrite with no version means "this key is created for the first time". Dropping it
// from the read set turns it into a BlindWrite on re-serialization, which changes both the
// bytes and their meaning to the committer.
func TestRWSet_VersionlessReadWriteRoundTrips(t *testing.T) {
	t.Parallel()
	first, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{
		NsId:       "ns1",
		NsVersion:  7,
		ReadWrites: []*applicationpb.ReadWrite{{Key: []byte("new-key"), Version: nil, Value: []byte("val")}},
	}}})
	require.NoError(t, err)

	m := vault.NewMarshaller()
	rws, nsVersions, err := m.RWSetFromBytes(first)
	require.NoError(t, err)
	second, err := m.Marshal("tx1", rws, nsVersions)
	require.NoError(t, err)

	var back applicationpb.Tx
	require.NoError(t, proto.Unmarshal(second, &back))
	require.Len(t, back.GetNamespaces()[0].GetReadWrites(), 1,
		"a versionless ReadWrite must stay a ReadWrite, not degrade to a BlindWrite")
	require.Equal(t, first, second)
}
