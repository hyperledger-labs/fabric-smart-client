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

type fakeHasher struct {
	hashFunc    func(msg []byte, opts bccsp.HashOpts) ([]byte, error)
	getHashFunc func(opts bccsp.HashOpts) (hash.Hash, error)
}

func (s *fakeHasher) Hash(msg []byte, opts bccsp.HashOpts) ([]byte, error) {
	return s.hashFunc(msg, opts)
}

func (s *fakeHasher) GetHash(opts bccsp.HashOpts) (hash.Hash, error) {
	return s.getHashFunc(opts)
}

func TestNewProvider(t *testing.T) {
	t.Parallel()

	p := NewProvider()
	require.NotNil(t, p)
	require.NotNil(t, p.hasher)
}

func TestProviderHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  []byte
	}{
		{
			name: "non-empty message",
			msg:  []byte("hello world"),
		},
		{
			name: "empty message",
			msg:  nil,
		},
		{
			name: "binary message",
			msg:  []byte{0x00, 0x01, 0x02, 0xff},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := NewProvider()

			digest, err := p.Hash(tt.msg)
			require.NoError(t, err)

			expected := sha256.Sum256(tt.msg)
			require.Equal(t, expected[:], digest)
		})
	}
}

func TestProviderGetHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  []byte
	}{
		{
			name: "non-empty message",
			msg:  []byte("hello world"),
		},
		{
			name: "empty message",
			msg:  nil,
		},
		{
			name: "binary message",
			msg:  []byte{0x00, 0x01, 0x02, 0xff},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := NewProvider()
			h := p.GetHash()
			require.NotNil(t, h)
			require.Equal(t, sha256.Size, h.Size())
			require.Equal(t, sha256.BlockSize, h.BlockSize())

			n, err := h.Write(tt.msg)
			require.NoError(t, err)
			require.Equal(t, len(tt.msg), n)

			expected := sha256.Sum256(tt.msg)
			require.Equal(t, expected[:], h.Sum(nil))

			h.Reset()
			emptyDigest := sha256.Sum256(nil)
			require.Equal(t, emptyDigest[:], h.Sum(nil))
		})
	}
}

func TestProviderHashPanicsOnHasherError(t *testing.T) {
	t.Parallel()

	p := &provider{
		hasher: &fakeHasher{
			hashFunc: func(msg []byte, opts bccsp.HashOpts) ([]byte, error) {
				require.IsType(t, &bccsp.SHA256Opts{}, opts)
				require.Equal(t, []byte("hello world"), msg)
				return nil, errors.New("hash failed")
			},
			getHashFunc: func(opts bccsp.HashOpts) (hash.Hash, error) {
				t.Fatal("GetHash should not be called")
				return nil, nil
			},
		},
	}

	require.PanicsWithError(t, "failed computing SHA256 on [68 65 6c 6c 6f 20 77 6f 72 6c 64]", func() {
		_, _ = p.Hash([]byte("hello world"))
	})
}

func TestProviderGetHashPanicsOnHasherError(t *testing.T) {
	t.Parallel()

	p := &provider{
		hasher: &fakeHasher{
			hashFunc: func(msg []byte, opts bccsp.HashOpts) ([]byte, error) {
				t.Fatal("Hash should not be called")
				return nil, nil
			},
			getHashFunc: func(opts bccsp.HashOpts) (hash.Hash, error) {
				require.IsType(t, &bccsp.SHA256Opts{}, opts)
				return nil, errors.New("get hash failed")
			},
		},
	}

	require.PanicsWithError(t, "failed getting SHA256", func() {
		_ = p.GetHash()
	})
}
