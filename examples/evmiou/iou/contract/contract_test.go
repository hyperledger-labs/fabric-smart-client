/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package contract

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/crypto"
)

func addr(low byte) client.Address {
	var a client.Address
	a[client.AddressLength-1] = low

	return a
}

func sig(low byte) []byte {
	s := make([]byte, 65)
	s[0] = low
	s[64] = 27

	return s
}

func TestBytecodeEmbedded(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, bytecode, "the embedded bytecode must not be empty")

	decoded, err := hex.DecodeString(strings.TrimSpace(bytecodeHex))
	require.NoError(t, err, "IOU.bin must decode as hex")
	assert.Equal(t, decoded, bytecode)
}

func TestSelectors(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		want := crypto.Keccak256([]byte("create(bytes32,uint256,address,address,bytes,bytes,bytes)"))[:4]
		data := EncodeCreate(Create{Borrower: addr(1), Lender: addr(2)}, sig(1), sig(2), sig(3))
		assert.Equal(t, want, data[:4])
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		want := crypto.Keccak256([]byte("update(bytes32,uint256,uint256,bytes,bytes,bytes)"))[:4]
		data := EncodeUpdate(Update{}, sig(1), sig(2), sig(3))
		assert.Equal(t, want, data[:4])
	})

	t.Run("get", func(t *testing.T) {
		t.Parallel()

		want := crypto.Keccak256([]byte("get(bytes32)"))[:4]
		data := EncodeGet([32]byte{})
		assert.Equal(t, want, data[:4])
	})
}

func TestEncodeCreateLayout(t *testing.T) {
	t.Parallel()

	data := EncodeCreate(Create{ID: [32]byte{0x01}, Amount: 10, Borrower: addr(1), Lender: addr(2)},
		sig(1), sig(2), sig(3))

	wantLen := 4 + 32*7 + 3*(32+96)
	require.Len(t, data, wantLen)

	// Heads: selector(4) + id, amount, borrower, lender (4 static words) + 3 offset words.
	headWord := func(i int) uint64 {
		return binary.BigEndian.Uint64(data[4+i*32+24 : 4+i*32+32])
	}
	assert.Equal(t, uint64(0xe0), headWord(4), "sigBorrower offset")
	assert.Equal(t, uint64(0x160), headWord(5), "sigLender offset")
	assert.Equal(t, uint64(0x1e0), headWord(6), "sigApprover offset")
}

func TestEncodeUpdateLayout(t *testing.T) {
	t.Parallel()

	data := EncodeUpdate(Update{ID: [32]byte{0x01}, Amount: 5, Version: 1}, sig(1), sig(2), sig(3))

	wantLen := 4 + 32*6 + 3*128
	require.Len(t, data, wantLen)

	headWord := func(i int) uint64 {
		return binary.BigEndian.Uint64(data[4+i*32+24 : 4+i*32+32])
	}
	assert.Equal(t, uint64(0xc0), headWord(3), "sigBorrower offset")
	assert.Equal(t, uint64(0x140), headWord(4), "sigLender offset")
	assert.Equal(t, uint64(0x1c0), headWord(5), "sigApprover offset")
}

func TestDecodeGetRoundTrip(t *testing.T) {
	t.Parallel()

	b, l := addr(0xaa), addr(0xbb)
	ret := make([]byte, 0, 128)
	ret = append(ret, make([]byte, 24)...)
	ret = append(ret, 0, 0, 0, 0, 0, 0, 0, 10) // amount = 10
	ret = append(ret, make([]byte, 12)...)
	ret = append(ret, b.Bytes()...)
	ret = append(ret, make([]byte, 12)...)
	ret = append(ret, l.Bytes()...)
	ret = append(ret, make([]byte, 24)...)
	ret = append(ret, 0, 0, 0, 0, 0, 0, 0, 1) // version = 1

	got, err := DecodeGet(ret)
	require.NoError(t, err)
	assert.Equal(t, IOU{Amount: 10, Borrower: b, Lender: l, Version: 1}, got)

	_, err = DecodeGet(ret[:127])
	assert.Error(t, err)
}

// TestHashStructPinned guards Create.HashStruct() against accidental changes with a golden value
// recorded the first time this test passed. The integration suite proves this matches the
// Solidity contract's own hashStruct computation on a real node.
func TestHashStructPinned(t *testing.T) {
	t.Parallel()

	c := Create{ID: [32]byte{0x01}, Amount: 10, Borrower: addr(1), Lender: addr(2)}
	got := c.HashStruct()
	assert.Equal(t, "bf77dbd25654eee4d09724bca07edaff76eda4b59e1824ceab0dfc002122c202", hex.EncodeToString(got[:]))
}
