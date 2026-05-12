/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package crypto

import (
	"crypto/sha256"
	"errors"
	"hash"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/stretchr/testify/require"
)

// mockFactoryErr implements the minimal subset of the factory
// interface required to exercise provider panic branches.
type mockFactoryErr struct{}

func (m *mockFactoryErr) Hash(msg []byte, opts bccsp.HashOpts) ([]byte, error) {
	return nil, errors.New("boom")
}
func (m *mockFactoryErr) GetHash(opts bccsp.HashOpts) (hash.Hash, error) {
	return nil, errors.New("boom")
}

func TestNewProvider(t *testing.T) {
	p := NewProvider()
	require.NotNil(t, p)
}

func TestHash(t *testing.T) {
	p := NewProvider()

	msg := []byte("hello world")
	got, err := p.Hash(msg)
	require.NoError(t, err)
	require.Len(t, got, sha256.Size)

	// compare against stdlib sha256
	exp := sha256.Sum256(msg)
	require.Equal(t, exp[:], got)
}

func TestHash_NilInput(t *testing.T) {
	p := NewProvider()

	got, err := p.Hash(nil)
	require.NoError(t, err)
	require.Len(t, got, sha256.Size)

	exp := sha256.Sum256(nil)
	require.Equal(t, exp[:], got)
}

func TestHash_Table(t *testing.T) {
	p := NewProvider()

	tests := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte(""),
		nil,
	}

	for _, tt := range tests {
		got, err := p.Hash(tt)
		require.NoError(t, err)
		require.Len(t, got, sha256.Size)

		exp := sha256.Sum256(tt)
		require.Equal(t, exp[:], got)
	}
}

func TestGetHash(t *testing.T) {
	p := NewProvider()
	h := p.GetHash()
	require.NotNil(t, h)

	msg := []byte("abc")
	_, err := h.Write(msg)
	require.NoError(t, err)
	got := h.Sum(nil)

	exp := sha256.Sum256(msg)
	require.Equal(t, exp[:], got)
}

func TestGetHash_NewInstance(t *testing.T) {
	p := NewProvider()

	h1 := p.GetHash()
	h2 := p.GetHash()
	require.NotSame(t, h1, h2)
}

func TestHash_PanicOnFactoryError(t *testing.T) {
	p := NewProvider()

	// replace factory with a mock that returns an error
	orig := getDefaultFactory
	defer func() { getDefaultFactory = orig }()

	getDefaultFactory = func() bccspFactory { return &mockFactoryErr{} }

	require.Panics(t, func() { _, _ = p.Hash([]byte("abc")) })
}

func TestGetHash_PanicOnFactoryError(t *testing.T) {
	p := NewProvider()

	orig := getDefaultFactory
	defer func() { getDefaultFactory = orig }()

	getDefaultFactory = func() bccspFactory { return &mockFactoryErr{} }

	require.Panics(t, func() { _ = p.GetHash() })
}
