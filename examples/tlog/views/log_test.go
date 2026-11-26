/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
)

// signedRequest returns a protojson request for artifact whose signature
// covers signed.
func signedRequest(t *testing.T, artifact, signed []byte) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	signedDigest := sha256.Sum256(signed)
	sig, err := ecdsa.SignASN1(rand.Reader, key, signedDigest[:])
	require.NoError(t, err)
	digest := sha256.Sum256(artifact)
	raw, err := protojson.Marshal(translog.NewRequest(digest[:], sig, pub))
	require.NoError(t, err)
	return raw
}

func TestValidateLogTx(t *testing.T) {
	t.Parallel()

	const n = 5
	request := signedRequest(t, []byte("artifact"), []byte("artifact"))
	req, err := decodeRequest(request)
	require.NoError(t, err)
	body, err := translog.CanonicalEntry(req)
	require.NoError(t, err)
	good := map[string][]byte{EntryKey(n): body, SizeKey: formatSize(n + 1)}
	with := func(edit func(map[string][]byte)) map[string][]byte {
		w := maps.Clone(good)
		edit(w)
		return w
	}

	sizeRead := []string{SizeKey}
	// v is the version of size; a missing key has no version.
	v := []byte{7}
	for _, tc := range []struct {
		name               string
		commands           []string
		reads              []string
		readVer, ledgerVer []byte
		writes             map[string][]byte
		request            []byte
		wantErr            string
	}{
		{"accepts", []string{"log"}, sizeRead, v, v, good, request, ""},
		{"accepts size read as missing while missing", []string{"log"}, sizeRead, nil, nil, good, request, ""},
		{"no read of size", []string{"log"}, nil, nil, v, good, request, "no read of [size]"},
		{"read at a newer version", []string{"log"}, sizeRead, []byte{8}, v, good, request, "read at version"},
		{"read as missing, but size exists", []string{"log"}, sizeRead, nil, []byte{0}, good, request, "read at version"},
		{"read at version 0, but size is missing", []string{"log"}, sizeRead, []byte{0}, nil, good, request, "read at version"},
		{"wrong command", []string{"delete"}, sizeRead, v, v, good, request, "single command"},
		{"extra command", []string{"log", "log"}, sizeRead, v, v, good, request, "single command"},
		{"third write", []string{"log"}, sizeRead, v, v, with(func(w map[string][]byte) { w["other"] = []byte("x") }), request, "expected 2 writes"},
		{"entry at the wrong index", []string{"log"}, sizeRead, v, v, with(func(w map[string][]byte) {
			delete(w, EntryKey(n))
			w[EntryKey(n-1)] = body
		}), request, "no write to [entry_5]"},
		{"other entry bytes", []string{"log"}, sizeRead, v, v, with(func(w map[string][]byte) { w[EntryKey(n)] = append([]byte{' '}, body...) }), request, "not the entry"},
		{"size not n+1", []string{"log"}, sizeRead, v, v, with(func(w map[string][]byte) { w[SizeKey] = formatSize(n + 2) }), request, "size written as [7]"},
		{"missing request", []string{"log"}, sizeRead, v, v, good, nil, "decoding the request"},
		{"garbled request", []string{"log"}, sizeRead, v, v, good, []byte("{not json"), "decoding the request"},
		{"bad signature", []string{"log"}, sizeRead, v, v, good, signedRequest(t, []byte("artifact"), []byte("other")), "verif"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateLogTx(tc.commands, tc.reads, tc.readVer, tc.ledgerVer, tc.writes, tc.request, n)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
