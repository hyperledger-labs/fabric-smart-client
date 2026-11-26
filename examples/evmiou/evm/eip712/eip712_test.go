/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Adapted from Panurus x/token/services/network/evm/eip712/eip712_test.go at commit f144b2b7f:
// Domain no longer carries a frozen name/version, and Digest takes a caller-supplied struct hash
// instead of hashing a Token SDK StateDelta, so the StateDelta-specific fixtures and golden values
// do not carry over. What does carry over is the separator and digest behaviour itself: the
// signing domain actually covers every field, and the digest is a deterministic, pure function of
// the domain and the struct hash.

package eip712

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
)

// fixtureDomain is the EIP-712 domain the IOU demo signs over: name "FSC IOU", version "1", the
// Besu dev chain id, and a fixed contract address.
func fixtureDomain(t *testing.T) Domain {
	t.Helper()
	addr, err := client.HexToAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)

	return Domain{Name: "FSC IOU", Version: "1", ChainID: big.NewInt(1337), VerifyingContract: addr}
}

func b32(v byte) [32]byte {
	var b [32]byte
	for i := range b {
		b[i] = v
	}

	return b
}

func TestDigestDeterministic(t *testing.T) {
	t.Parallel()

	d := fixtureDomain(t)
	structHash := b32(0x11)
	first, second := Digest(d, structHash), Digest(d, structHash)
	assert.Equal(t, first, second)
}

// TestDigestSensitivity guards that the digest actually covers every domain field and the struct
// hash: mutating any of them must change the digest, otherwise a signature over one domain or
// struct could be replayed as a signature over another.
func TestDigestSensitivity(t *testing.T) {
	t.Parallel()

	d := fixtureDomain(t)
	structHash := b32(0x11)
	base := Digest(d, structHash)

	t.Run("struct hash", func(t *testing.T) {
		t.Parallel()

		assert.NotEqual(t, base, Digest(d, b32(0x22)))
	})
	t.Run("name", func(t *testing.T) {
		t.Parallel()

		mutated := d
		mutated.Name = "something else"
		assert.NotEqual(t, base, Digest(mutated, structHash))
	})
	t.Run("version", func(t *testing.T) {
		t.Parallel()

		mutated := d
		mutated.Version = "2"
		assert.NotEqual(t, base, Digest(mutated, structHash))
	})
	t.Run("chain id", func(t *testing.T) {
		t.Parallel()

		mutated := d
		mutated.ChainID = big.NewInt(1)
		assert.NotEqual(t, base, Digest(mutated, structHash))
	})
	t.Run("verifying contract", func(t *testing.T) {
		t.Parallel()

		mutated := d
		mutated.VerifyingContract = client.Address{0x01}
		assert.NotEqual(t, base, Digest(mutated, structHash))
	})
}

// TestSeparatorNilChainID guards that a nil ChainID does not panic (it is treated as zero); a valid
// chain id is enforced by the caller.
func TestSeparatorNilChainID(t *testing.T) {
	t.Parallel()

	addr, err := client.HexToAddress("0x1234567890123456789012345678901234567890")
	require.NoError(t, err)
	d := Domain{Name: "FSC IOU", Version: "1", ChainID: nil, VerifyingContract: addr}
	assert.NotPanics(t, func() { _ = d.Separator() })
}

// TestSeparatorDeterministic pins that the separator is a pure function of the domain, which the
// digest's own determinism depends on.
func TestSeparatorDeterministic(t *testing.T) {
	t.Parallel()

	d := fixtureDomain(t)
	first, second := d.Separator(), d.Separator()
	assert.Equal(t, first, second)
}
