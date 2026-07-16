/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

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
			m.NsInfo = nsInfo

			first, err := m.Marshal("tx1", &rws)
			require.NoError(t, err)

			const attempts = 30
			for i := range attempts {
				out, err := m.Marshal("tx1", &rws)
				require.NoError(t, err)
				require.Equal(t, first, out,
					"Marshal produced different bytes for the exact same ReadWriteSet on attempt %d/%d; "+
						"this indicates Marshal is not deterministic (likely due to unsorted map iteration)", i, attempts)
			}
		})
	}
}
