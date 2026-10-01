/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commonvault "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

// newTestRWSet returns an empty RWSet for tx1 backed by qs and mds.
func newTestRWSet(t *testing.T, qs *mockQueryService, mds fdriver.MetadataService) driver.RWSet {
	t.Helper()
	rws, err := vault.NewVault(qs, mds).NewRWSet(context.Background(), "tx1")
	require.NoError(t, err)
	return rws
}

func TestRWSet_IsValid(t *testing.T) {
	t.Parallel()

	t.Run("empty rwset issues no query", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		rws := newTestRWSet(t, qs, nil)

		require.NoError(t, rws.IsValid())
		require.Equal(t, int32(0), qs.getStatesCount.Load())
	})

	t.Run("query error is wrapped with the txID", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		rws := newTestRWSet(t, qs, nil)
		require.NoError(t, rws.AddReadAt("ns1", "key1", vault.MarshalVersion(1)))

		qs.getStatesErr = errors.New("simulated failure")
		err := rws.IsValid()
		require.ErrorContains(t, err, "failed to validate rwset for tx tx1")
		require.ErrorContains(t, err, "simulated failure")
	})

	t.Run("key deleted after the read", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("ns1", "key1", []byte("val1"), 1)
		rws := newTestRWSet(t, qs, nil)
		_, err := rws.GetState("ns1", "key1")
		require.NoError(t, err)

		delete(qs.states["ns1"], "key1")
		require.ErrorContains(t, rws.IsValid(), "key key1 in namespace ns1 was deleted")
	})

	t.Run("key updated after the read", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("ns1", "key1", []byte("val1"), 1)
		rws := newTestRWSet(t, qs, nil)
		_, err := rws.GetState("ns1", "key1")
		require.NoError(t, err)

		qs.setState("ns1", "key1", []byte("val2"), 2)
		require.ErrorContains(t, rws.IsValid(), "version mismatch for key key1 in namespace ns1")
	})

	t.Run("key created after an absent read", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		rws := newTestRWSet(t, qs, nil)
		// A nil version records that the key is expected not to exist.
		require.NoError(t, rws.AddReadAt("ns1", "key1", nil))
		require.NoError(t, rws.IsValid())

		qs.setState("ns1", "key1", []byte("val1"), 1)
		require.ErrorContains(t, rws.IsValid(), "version mismatch for key key1 in namespace ns1")
	})
}

// A failed namespace pin must surface from every entry point and leave the RWSet untouched.
func TestRWSet_PinFailureLeavesRWSetUnchanged(t *testing.T) {
	t.Parallel()

	ops := map[string]func(rws driver.RWSet) error{
		"AddReadAt": func(rws driver.RWSet) error { return rws.AddReadAt("ns1", "key1", vault.MarshalVersion(1)) },
		"SetState":  func(rws driver.RWSet) error { return rws.SetState("ns1", "key1", []byte("val")) },
		"DeleteState": func(rws driver.RWSet) error {
			return rws.DeleteState("ns1", "key1")
		},
		"SetStateMetadata": func(rws driver.RWSet) error {
			return rws.SetStateMetadata("ns1", "key1", driver.Metadata{"m": []byte("v")})
		},
		// The key lookup succeeds; only the _meta lookup that follows it fails.
		"GetState": func(rws driver.RWSet) error {
			_, err := rws.GetState("ns1", "key1")
			return err
		},
	}

	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			qs := newMockQueryService()
			qs.setState("ns1", "key1", []byte("val1"), 1)
			qs.getStatesErr = errors.New("simulated lookup failure")
			rws := newTestRWSet(t, qs, nil)

			require.ErrorContains(t, op(rws), "simulated lookup failure")
			require.Equal(t, 0, rws.NumReads("ns1"))
			require.Equal(t, 0, rws.NumWrites("ns1"))
			require.Empty(t, rws.Namespaces())
		})
	}
}

func TestRWSet_GetState(t *testing.T) {
	t.Parallel()

	t.Run("FromIntermediate does not query", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("ns1", "key1", []byte("val1"), 1)
		rws := newTestRWSet(t, qs, nil)

		val, err := rws.GetState("ns1", "key1", driver.FromIntermediate)
		require.NoError(t, err)
		require.Nil(t, val)
		require.Equal(t, 0, rws.NumReads("ns1"))
	})

	// A miss is a read dependency on the key's absence: it enters the read set with a nil
	// version and serializes as a versionless read.
	t.Run("remote miss records an absent read", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("_meta", "ns1", nil, 7)
		rws := newTestRWSet(t, qs, nil)

		val, err := rws.GetState("ns1", "missing")
		require.NoError(t, err)
		require.Nil(t, val)
		require.Equal(t, 1, rws.NumReads("ns1"))
		require.NoError(t, rws.IsValid())

		raw, err := rws.Bytes()
		require.NoError(t, err)
		var tx applicationpb.Tx
		require.NoError(t, proto.Unmarshal(raw, &tx))
		require.Len(t, tx.GetNamespaces(), 1)
		require.Equal(t, uint64(7), tx.GetNamespaces()[0].GetNsVersion())
		require.Len(t, tx.GetNamespaces()[0].GetReadsOnly(), 1)
		require.Nil(t, tx.GetNamespaces()[0].GetReadsOnly()[0].Version)
	})

	// Checking that a key is free and then creating it must conflict with a concurrent
	// creation of the same key.
	t.Run("key created after a missed read", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		rws := newTestRWSet(t, qs, nil)
		val, err := rws.GetState("ns1", "key1")
		require.NoError(t, err)
		require.Nil(t, val)
		require.NoError(t, rws.SetState("ns1", "key1", []byte("mine")))

		raw, err := rws.Bytes()
		require.NoError(t, err)
		var tx applicationpb.Tx
		require.NoError(t, proto.Unmarshal(raw, &tx))
		require.Len(t, tx.GetNamespaces()[0].GetReadWrites(), 1, "the create must be conditional on absence")
		require.Nil(t, tx.GetNamespaces()[0].GetReadWrites()[0].Version)
		require.Empty(t, tx.GetNamespaces()[0].GetBlindWrites())

		qs.setState("ns1", "key1", []byte("theirs"), 1)
		require.ErrorContains(t, rws.IsValid(), "version mismatch for key key1 in namespace ns1")
	})

	t.Run("re-read after a missed key was created", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		rws := newTestRWSet(t, qs, nil)
		_, err := rws.GetState("ns1", "key1")
		require.NoError(t, err)

		qs.setState("ns1", "key1", []byte("theirs"), 1)
		_, err = rws.GetState("ns1", "key1")
		require.ErrorContains(t, err, "invalid read [ns1:key1]")
	})

	t.Run("re-read at the recorded version", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("ns1", "key1", []byte("val1"), 1)
		rws := newTestRWSet(t, qs, nil)

		for range 2 {
			val, err := rws.GetState("ns1", "key1")
			require.NoError(t, err)
			require.Equal(t, []byte("val1"), val)
		}
		require.Equal(t, 1, rws.NumReads("ns1"))
	})

	// The caller acted on the first value, so a later commit must not silently move the
	// recorded read version forward and let IsValid accept the stale simulation.
	t.Run("re-read after a commit changed the key", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("ns1", "key1", []byte("val1"), 1)
		rws := newTestRWSet(t, qs, nil)
		_, err := rws.GetState("ns1", "key1")
		require.NoError(t, err)

		qs.setState("ns1", "key1", []byte("val2"), 2)
		_, err = rws.GetState("ns1", "key1")
		require.ErrorContains(t, err, "invalid read [ns1:key1]")
		require.ErrorContains(t, rws.IsValid(), "version mismatch for key key1", "the original read version must be kept")
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.getStateErr = errors.New("simulated failure")
		rws := newTestRWSet(t, qs, nil)

		_, err := rws.GetState("ns1", "key1")
		require.ErrorContains(t, err, "failed to get state for namespace=ns1, key=key1")
		require.ErrorContains(t, err, "simulated failure")
	})
}

func TestRWSet_GetDirectState(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("ns1", "key1", []byte("val1"), 1)
	rws := newTestRWSet(t, qs, nil)

	val, err := rws.GetDirectState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, []byte("val1"), val)
	require.Equal(t, 0, rws.NumReads("ns1"), "a direct read bypasses the read set")

	val, err = rws.GetDirectState("ns1", "missing")
	require.NoError(t, err)
	require.Nil(t, val)

	qs.getStateErr = errors.New("simulated failure")
	_, err = rws.GetDirectState("ns1", "key1")
	require.ErrorContains(t, err, "failed to get direct state for namespace=ns1, key=key1")
}

func TestRWSet_GetReadAtMissAndError(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	rws := newTestRWSet(t, qs, nil)
	require.NoError(t, rws.AddReadAt("ns1", "key1", vault.MarshalVersion(1)))

	// The read was recorded locally, but the key does not exist remotely.
	key, val, err := rws.GetReadAt("ns1", 0)
	require.NoError(t, err)
	require.Equal(t, "key1", key)
	require.Nil(t, val)

	qs.getStateErr = errors.New("simulated failure")
	_, _, err = rws.GetReadAt("ns1", 0)
	require.ErrorContains(t, err, "failed to get read at index 0 for namespace=ns1")
}

func TestRWSet_GetReadVersion(t *testing.T) {
	t.Parallel()
	rws := newTestRWSet(t, newMockQueryService(), nil)
	require.NoError(t, rws.AddReadAt("ns1", "key1", vault.MarshalVersion(1)))

	version, err := rws.GetReadVersion("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, vault.MarshalVersion(1), version)

	_, err = rws.GetReadVersion("ns1", "key2")
	require.ErrorContains(t, err, "no read of key key2 for namespace ns1")
}

// GetStateMetadata falls back to the field-mapping store only for a committed, non-empty
// value; every miss along the way is reported as "no metadata" rather than as an error.
func TestRWSet_GetStateMetadataFallback(t *testing.T) {
	t.Parallel()

	committed := []byte("committed-value")
	digest := sha256.Sum256(committed)

	tests := []struct {
		name  string
		setup func(qs *mockQueryService, mds *mockMDS)
		nilMD bool
		opt   driver.GetStateOpt // zero value FromStorage takes the fallback path like FromBoth
	}{
		{name: "FromIntermediate", opt: driver.FromIntermediate, setup: func(qs *mockQueryService, mds *mockMDS) {
			qs.setState("ns1", "key1", committed, 1)
			mds.fieldMappings = map[string]fdriver.TransientMap{string(digest[:]): {"fm": []byte("x")}}
		}},
		{name: "nil metadata service", nilMD: true, setup: func(qs *mockQueryService, _ *mockMDS) {
			qs.setState("ns1", "key1", committed, 1)
		}},
		{name: "no committed value", setup: func(*mockQueryService, *mockMDS) {}},
		{name: "empty committed value", setup: func(qs *mockQueryService, _ *mockMDS) {
			qs.setState("ns1", "key1", nil, 1)
		}},
		{name: "field mapping lookup fails", setup: func(qs *mockQueryService, mds *mockMDS) {
			qs.setState("ns1", "key1", committed, 1)
			mds.getFieldMappingErr = errors.New("no row")
		}},
		{name: "empty field mapping", setup: func(qs *mockQueryService, mds *mockMDS) {
			qs.setState("ns1", "key1", committed, 1)
			mds.fieldMappings = map[string]fdriver.TransientMap{string(digest[:]): {}}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			qs := newMockQueryService()
			mds := &mockMDS{}
			tc.setup(qs, mds)

			var ms fdriver.MetadataService = mds
			if tc.nilMD {
				ms = nil
			}
			meta, err := newTestRWSet(t, qs, ms).GetStateMetadata("ns1", "key1", tc.opt)
			require.NoError(t, err)
			require.Nil(t, meta)
		})
	}

	t.Run("committed state query error is wrapped", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.getStateErr = errors.New("simulated failure")

		_, err := newTestRWSet(t, qs, &mockMDS{}).GetStateMetadata("ns1", "key1")
		require.ErrorContains(t, err, "failed getting committed state for namespace=ns1, key=key1")
	})
}

func TestRWSet_NamespacesFromReadsAndMetaWrites(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()

	reads := newTestRWSet(t, qs, nil)
	require.NoError(t, reads.AddReadAt("ns1", "key1", vault.MarshalVersion(1)))
	require.Equal(t, []driver.Namespace{"ns1"}, reads.Namespaces())

	metaWrites := newTestRWSet(t, qs, nil)
	require.NoError(t, metaWrites.SetStateMetadata("ns2", "key1", driver.Metadata{"m": []byte("v")}))
	require.Equal(t, []driver.Namespace{"ns2"}, metaWrites.Namespaces())
}

func TestRWSet_Equals(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()

	build := func(t *testing.T, mutate func(rws driver.RWSet)) driver.RWSet {
		t.Helper()
		rws := newTestRWSet(t, qs, nil)
		require.NoError(t, rws.AddReadAt("ns1", "r", vault.MarshalVersion(1)))
		require.NoError(t, rws.SetState("ns1", "w", []byte("v")))
		require.NoError(t, rws.SetStateMetadata("ns1", "m", driver.Metadata{"k": []byte("v")}))
		require.NoError(t, rws.SetState("ns2", "w", []byte("v")))
		if mutate != nil {
			mutate(rws)
		}
		return rws
	}

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, build(t, nil).Equals("not an rwset"), "expected *rwSetWrapper, got string")
	})

	t.Run("self", func(t *testing.T) {
		t.Parallel()
		a := build(t, nil)
		require.NoError(t, a.Equals(a)) //nolint:gocritic // comparing with itself is the case under test
	})

	t.Run("equal pair both ways", func(t *testing.T) {
		t.Parallel()
		// Comparing in both directions runs both lock-ordering branches.
		a, b := build(t, nil), build(t, nil)
		require.NoError(t, a.Equals(b))
		require.NoError(t, b.Equals(a))
	})

	diffs := map[string]func(rws driver.RWSet){
		"reads": func(rws driver.RWSet) {
			_ = rws.AddReadAt("ns1", "r", vault.MarshalVersion(2))
		},
		"writes": func(rws driver.RWSet) {
			_ = rws.SetState("ns1", "w", []byte("other"))
		},
		"metadata writes": func(rws driver.RWSet) {
			_ = rws.SetStateMetadata("ns1", "m", driver.Metadata{"k": []byte("other")})
		},
	}
	for name, mutate := range diffs {
		t.Run("differing "+name, func(t *testing.T) {
			t.Parallel()
			a, b := build(t, nil), build(t, mutate)
			require.ErrorContains(t, a.Equals(b), "entries for [ns1] do not match")
			require.ErrorContains(t, a.Equals(b, "ns1"), "entries for [ns1] do not match")
			require.NoError(t, a.Equals(b, "ns2"), "a filter excluding ns1 must ignore the difference")
		})
	}

	// Pinned namespace versions are serialized as NsVersion, so RWSets that differ only
	// there are different transactions.
	t.Run("differing namespace versions", func(t *testing.T) {
		t.Parallel()
		qsB := newMockQueryService()
		qsB.setState("_meta", "ns1", nil, 2)
		a := newTestRWSet(t, qs, nil)
		b := newTestRWSet(t, qsB, nil)
		for _, rws := range []driver.RWSet{a, b} {
			require.NoError(t, rws.SetState("ns1", "k", []byte("v")))
			require.NoError(t, rws.SetState("ns2", "k", []byte("v")))
		}

		require.ErrorContains(t, a.Equals(b), "namespace version for [ns1] does not match")
		require.ErrorContains(t, b.Equals(a, "ns1"), "namespace version for [ns1] does not match")
		require.NoError(t, a.Equals(b, "ns2"))
	})

	// A pinned namespace with no reads or writes is not serialized, so it cannot make two
	// RWSets differ.
	t.Run("pinned namespace without reads or writes", func(t *testing.T) {
		t.Parallel()
		a := newTestRWSet(t, qs, nil)
		b := newTestRWSet(t, qs, nil)
		raw, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns2", NsVersion: 5}}})
		require.NoError(t, err)
		require.NoError(t, b.AppendRWSet(raw))

		require.NoError(t, a.Equals(b))
		require.NoError(t, b.Equals(a))
	})
}

func TestRWSet_Lifecycle(t *testing.T) {
	t.Parallel()
	rws := newTestRWSet(t, newMockQueryService(), nil)
	require.False(t, rws.IsClosed())
	rws.Done()
	require.False(t, rws.IsClosed())
}

func TestRWSet_RejectsMalformedBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	v := vault.NewVault(newMockQueryService(), nil)
	garbage := []byte("garbage")

	_, err := v.NewRWSetFromBytes(ctx, "tx1", garbage)
	require.ErrorContains(t, err, "failed to unmarshal rwset")

	_, err = v.InspectRWSet(ctx, garbage)
	require.ErrorContains(t, err, "failed to unmarshal rwset for inspection")

	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.ErrorContains(t, rws.AppendRWSet(garbage), "unmarshal tx from")
}

func TestRWSet_RejectsInvalidNamespace(t *testing.T) {
	t.Parallel()
	version := uint64(1)

	tests := map[string]*applicationpb.TxNamespace{
		"read only":   {NsId: "bad ns!", ReadsOnly: []*applicationpb.Read{{Key: []byte("k"), Version: &version}}},
		"blind write": {NsId: "bad ns!", BlindWrites: []*applicationpb.Write{{Key: []byte("k"), Value: []byte("v")}}},
		"read write":  {NsId: "bad ns!", ReadWrites: []*applicationpb.ReadWrite{{Key: []byte("k"), Version: &version, Value: []byte("v")}}},
	}

	for name, ns := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{ns}})
			require.NoError(t, err)

			_, err = vault.NewVault(newMockQueryService(), nil).NewRWSetFromBytes(context.Background(), "tx1", raw)
			require.ErrorContains(t, err, "namespace 'bad ns!' is invalid")
		})
	}
}

// A namespace whose _meta entry carries no version cannot be serialized, whether it holds
// writes or only reads; encoding it as version 0 would misstate the namespace version.
func TestRWSet_BytesRejectsNilNamespaceVersion(t *testing.T) {
	t.Parallel()

	ops := map[string]func(rws driver.RWSet) error{
		"write":     func(rws driver.RWSet) error { return rws.SetState("ns1", "key1", []byte("val1")) },
		"read only": func(rws driver.RWSet) error { return rws.AddReadAt("ns1", "key1", vault.MarshalVersion(1)) },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			qs := newMockQueryService()
			qs.states["_meta"] = map[driver.PKey]driver.VaultValue{"ns1": {}}
			rws := newTestRWSet(t, qs, nil)
			require.NoError(t, op(rws))

			_, err := rws.Bytes()
			require.ErrorContains(t, err, "nsVersion is nil for ns = [ns1]")
		})
	}
}

// Append validates the whole payload first, so a rejected payload leaves the RWSet as it
// was: still serializable, and without the namespaces that preceded the invalid one.
func TestRWSet_FailedAppendLeavesRWSetUnchanged(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	rws := newTestRWSet(t, qs, nil)
	require.NoError(t, rws.SetState("ns1", "key1", []byte("val1")))
	before, err := rws.Bytes()
	require.NoError(t, err)

	raw, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{
		{NsId: "ok", BlindWrites: []*applicationpb.Write{{Key: []byte("k"), Value: []byte("v")}}},
		{NsId: "bad ns!", BlindWrites: []*applicationpb.Write{{Key: []byte("k"), Value: []byte("v")}}},
	}})
	require.NoError(t, err)

	require.ErrorContains(t, rws.AppendRWSet(raw), "namespace 'bad ns!' is invalid")
	require.Equal(t, 0, rws.NumWrites("ok"))
	after, err := rws.Bytes()
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestMarshal_RequiresNamespaceInfo(t *testing.T) {
	t.Parallel()
	m := vault.NewMarshaller()

	writes := commonvault.EmptyRWSet()
	require.NoError(t, writes.WriteSet.Add("ns1", "key1", []byte("val1")))
	_, err := m.Marshal("tx1", &writes, nil)
	require.ErrorContains(t, err, "nsInfo does not contain entry for ns = [ns1]")

	reads := commonvault.EmptyRWSet()
	reads.ReadSet.Add("ns1", "key1", vault.MarshalVersion(1))
	_, err = m.Marshal("tx1", &reads, nil)
	require.ErrorContains(t, err, "nsInfo does not contain entry for ns = [ns1]")
}

// Read versions survive serialization, both for plain reads and for read-writes.
func TestRWSet_VersionedReadsRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	qs := newMockQueryService()
	qs.setState("ns1", "read", []byte("r"), 3)
	qs.setState("ns1", "readwrite", []byte("rw"), 5)
	v := vault.NewVault(qs, nil)

	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	_, err = rws.GetState("ns1", "read")
	require.NoError(t, err)
	_, err = rws.GetState("ns1", "readwrite")
	require.NoError(t, err)
	require.NoError(t, rws.SetState("ns1", "readwrite", []byte("new")))

	raw, err := rws.Bytes()
	require.NoError(t, err)

	var tx applicationpb.Tx
	require.NoError(t, proto.Unmarshal(raw, &tx))
	require.Len(t, tx.GetNamespaces(), 1)
	require.Equal(t, uint64(3), tx.GetNamespaces()[0].GetReadsOnly()[0].GetVersion())
	require.Equal(t, uint64(5), tx.GetNamespaces()[0].GetReadWrites()[0].GetVersion())

	restored, err := v.NewRWSetFromBytes(ctx, "tx1", raw)
	require.NoError(t, err)
	require.NoError(t, rws.Equals(restored), "restored reads must carry the original versions")
	require.NoError(t, restored.IsValid())

	qs.setState("ns1", "read", []byte("r2"), 4)
	require.ErrorContains(t, restored.IsValid(), "version mismatch for key read")
}
