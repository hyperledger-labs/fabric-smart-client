/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	commonvault "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

const (
	numNamespaces       = 8
	numKeysPerNamespace = 16
)

// buildRWSet populates a ReadWriteSet according to mode:
//   - "mixed": every key gets both a write and a read (read-write entries)
//   - "reads-only": every key gets only a read, no writes at all
//   - "writes-only": every key gets only a write (blind write), no reads at all
func buildRWSet(t *testing.T, mode string) (commonvault.ReadWriteSet, map[string][]byte) {
	t.Helper()

	rws := commonvault.EmptyRWSet()
	nsInfo := map[string][]byte{}

	for n := range numNamespaces {
		ns := fmt.Sprintf("ns%d", n)
		nsInfo[ns] = vault.MarshalVersion(uint64(n))

		for k := range numKeysPerNamespace {
			key := fmt.Sprintf("key%d", k)
			switch mode {
			case "mixed":
				require.NoError(t, rws.WriteSet.Add(ns, key, fmt.Appendf(nil, "value-%d-%d", n, k)))
				rws.ReadSet.Add(ns, key, vault.MarshalVersion(uint64(k)))
			case "reads-only":
				rws.ReadSet.Add(ns, key, vault.MarshalVersion(uint64(k)))
			case "writes-only":
				require.NoError(t, rws.WriteSet.Add(ns, key, fmt.Appendf(nil, "value-%d-%d", n, k)))
			default:
				t.Fatalf("unknown mode %q", mode)
			}
		}
	}

	return rws, nsInfo
}

// TestMarshal_Deterministic confirms that Marshaller.Marshal produces identical
// bytes when called repeatedly with the exact same input. Marshal builds its
// output by iterating over Go maps (namespaceSet, readSet, writeSet,
// readWriteSet) without ever sorting the keys, and Go intentionally
// randomizes map iteration order across separate range statements. As a
// result the order in which namespaces/keys are appended to the output
// proto's repeated fields can change from call to call, producing different
// serialized bytes for logically identical input.
//
// This is checked separately for read-write, reads-only, and writes-only
// RWSets since Marshal iterates readSet, writeSet, and readWriteSet as three
// independent maps, and the bug could in principle affect one without
// affecting the others.
func TestMarshal_Deterministic(t *testing.T) {
	t.Parallel()

	modes := []string{"mixed", "reads-only", "writes-only"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			rws, nsInfo := buildRWSet(t, mode)

			m := vault.NewMarshaller()

			first, err := m.Marshal("tx1", &rws, nsInfo)
			require.NoError(t, err)

			const attempts = 30
			for i := range attempts {
				out, err := m.Marshal("tx1", &rws, nsInfo)
				require.NoError(t, err)
				require.Equal(t, first, out,
					"Marshal produced different bytes for the exact same ReadWriteSet on attempt %d/%d; "+
						"this indicates Marshal is not deterministic (likely due to unsorted map iteration)", i, attempts)
			}
		})
	}
}

// withUnknownField returns m with an unknown varint field appended to its wire form.
func withUnknownField[M proto.Message](m M) M {
	m.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 1))
	return m
}

func TestRWSetFromBytes_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	version := uint64(1)
	tests := map[string]proto.Message{
		// Shares no field numbers with applicationpb.Tx.
		"foreign message": &cb.ChannelHeader{Type: int32(cb.HeaderType_MESSAGE), ChannelId: "ch", TxId: "tx1"},
		// Field 1 is length-delimited, so the header parses as a TxNamespace; its
		// signature header is field 2, which TxNamespace declares as a varint.
		"foreign message with nested field": &cb.Payload{Header: &cb.Header{
			ChannelHeader:   []byte("ch"),
			SignatureHeader: []byte("sh"),
		}},
		"unknown top-level field": withUnknownField(&applicationpb.Tx{}),
		"unknown field in a read": &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{
			NsId:      "ns1",
			ReadsOnly: []*applicationpb.Read{withUnknownField(&applicationpb.Read{Key: []byte("k"), Version: &version})},
		}}},
		"unknown field in an endorsement identity": &applicationpb.Tx{Endorsements: []*applicationpb.Endorsements{{
			EndorsementsWithIdentity: []*applicationpb.EndorsementWithIdentity{{Identity: withUnknownField(&msppb.Identity{MspId: "org"})}},
		}}},
	}
	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := proto.Marshal(msg)
			require.NoError(t, err)
			_, _, err = (&vault.Marshaller{}).RWSetFromBytes(raw)
			require.ErrorContains(t, err, "unmarshal tx from")
			require.ErrorContains(t, err, "unknown fields in")
		})
	}
}

func TestRWSetFromBytes_AcceptsKnownFields(t *testing.T) {
	t.Parallel()
	version := uint64(3)
	tx := &applicationpb.Tx{
		Namespaces: []*applicationpb.TxNamespace{{
			NsId:        "ns1",
			NsVersion:   1,
			ReadsOnly:   []*applicationpb.Read{{Key: []byte("r"), Version: &version}},
			ReadWrites:  []*applicationpb.ReadWrite{{Key: []byte("rw"), Version: &version, Value: []byte("v")}},
			BlindWrites: []*applicationpb.Write{{Key: []byte("w"), Value: []byte("v")}},
		}},
		Endorsements: []*applicationpb.Endorsements{{
			EndorsementsWithIdentity: []*applicationpb.EndorsementWithIdentity{{
				Endorsement: []byte("sig"),
				Identity:    &msppb.Identity{MspId: "org", Creator: &msppb.Identity_Certificate{Certificate: []byte("cert")}},
			}},
		}},
		Metadata: [][]byte{[]byte("meta")},
	}
	raw, err := proto.Marshal(tx)
	require.NoError(t, err)
	rws, nsVersions, err := (&vault.Marshaller{}).RWSetFromBytes(raw)
	require.NoError(t, err)
	require.Len(t, rws.Reads["ns1"], 2)
	require.Len(t, rws.Writes["ns1"], 2)
	require.Contains(t, nsVersions, "ns1")

	// Empty input serializes a transaction without namespaces.
	for _, raw := range [][]byte{nil, {}} {
		rws, _, err := (&vault.Marshaller{}).RWSetFromBytes(raw)
		require.NoError(t, err)
		require.Empty(t, rws.Reads)
		require.Empty(t, rws.Writes)
	}
}
