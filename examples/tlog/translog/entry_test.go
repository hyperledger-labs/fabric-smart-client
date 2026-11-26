/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package translog_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"testing"

	pb "github.com/sigstore/rekor-tiles/v2/pkg/generated/protobuf"
	"github.com/sigstore/rekor-tiles/v2/pkg/types/hashedrekord"
	"github.com/stretchr/testify/require"
	"github.com/transparency-dev/merkle/rfc6962"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
)

// newSignedRequest signs digest with a fresh P-256 key and returns the
// resulting request.
func newSignedRequest(t *testing.T, digest []byte) *pb.HashedRekordRequestV002 {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	return translog.NewRequest(digest, sig, pubDER)
}

func TestCanonicalEntryMatchesRekor(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("hello"))
	req := newSignedRequest(t, digest[:])

	body, err := translog.CanonicalEntry(req)
	require.NoError(t, err)

	want, err := hashedrekord.ToEntryHash(digest[:], req.Signature)
	require.NoError(t, err)
	require.Equal(t, want, rfc6962.DefaultHasher.HashLeaf(body))
}

func TestCanonicalEntryDeterministic(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("hello"))
	req := newSignedRequest(t, digest[:])

	first, err := translog.CanonicalEntry(req)
	require.NoError(t, err)
	second, err := translog.CanonicalEntry(req)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestCanonicalEntryRejectsBadSignature(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("hello"))
	other := sha256.Sum256([]byte("goodbye"))
	// Sign a different digest than the one carried in the request.
	req := newSignedRequest(t, other[:])
	req.Digest = digest[:]

	_, err := translog.CanonicalEntry(req)
	require.Error(t, err)
}

func TestCanonicalEntryRejectsShortDigest(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("hello"))
	req := newSignedRequest(t, digest[:])
	req.Digest = digest[:20]

	_, err := translog.CanonicalEntry(req)
	require.Error(t, err)
}
