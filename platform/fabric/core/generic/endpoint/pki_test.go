/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/sha256"
	"encoding/pem"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	x509msp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
)

func TestPublicKeyExtractorExtractPublicKey_InvalidSerializedIdentity(t *testing.T) {
	t.Parallel()

	extractor := PublicKeyExtractor{}
	_, err := extractor.ExtractPublicKey([]byte("not-a-protobuf"))
	require.ErrorContains(t, err, "cannot parse invalid wire-format data")
}

func TestPublicKeyExtractorExtractPublicKey_X509Identity(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	rawIdentity, err := x509msp.Serialize("apple", certPath)
	require.NoError(t, err)

	extractor := PublicKeyExtractor{}
	publicKey, err := extractor.ExtractPublicKey(rawIdentity)
	require.NoError(t, err)
	require.NotNil(t, publicKey)
}

func TestPublicKeyExtractorExtractPublicKey_InvalidCertificatePayload(t *testing.T) {
	t.Parallel()

	si := &msp.SerializedIdentity{
		Mspid: "apple",
		IdBytes: pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: []byte("bad-cert-der"),
		}),
	}
	rawIdentity, err := proto.Marshal(si)
	require.NoError(t, err)

	extractor := PublicKeyExtractor{}
	_, err = extractor.ExtractPublicKey(rawIdentity)
	require.ErrorContains(t, err, "malformed certificate")
}

func TestPublicKeyExtractorExtractPublicKey_InvalidIdemixPayload(t *testing.T) {
	t.Parallel()

	si := &msp.SerializedIdentity{
		Mspid:   "idemix-msp",
		IdBytes: []byte("not-pem-and-not-idemix"),
	}
	rawIdentity, err := proto.Marshal(si)
	require.NoError(t, err)

	extractor := PublicKeyExtractor{}
	_, err = extractor.ExtractPublicKey(rawIdentity)
	require.ErrorContains(t, err, "cannot parse invalid wire-format data")
}

func TestPublicKeyExtractorExtractPublicKey_IdemixIdentity(t *testing.T) {
	t.Parallel()

	serialized := &msp.SerializedIdemixIdentity{
		NymX:  []byte("nym-x"),
		NymY:  []byte("nym-y"),
		Proof: []byte("proof"),
		Ou:    []byte("ou"),
		Role:  []byte("role"),
	}
	idemixRaw, err := proto.Marshal(serialized)
	require.NoError(t, err)

	si := &msp.SerializedIdentity{
		Mspid:   "idemix-msp",
		IdBytes: idemixRaw,
	}
	rawIdentity, err := proto.Marshal(si)
	require.NoError(t, err)

	extractor := PublicKeyExtractor{}
	publicKey, err := extractor.ExtractPublicKey(rawIdentity)
	require.NoError(t, err)

	hashBytes, ok := publicKey.([]byte)
	require.True(t, ok)

	h := sha256.New()
	h.Write(serialized.NymX)
	h.Write(serialized.NymY)
	h.Write(serialized.Proof)
	h.Write(serialized.Ou)
	h.Write(serialized.Role)
	require.Equal(t, h.Sum(nil), hashBytes)
}
