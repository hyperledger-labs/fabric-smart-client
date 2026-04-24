/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	msp2 "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

func TestDeserializer_DeserializeVerifier(t *testing.T) {
	t.Parallel()
	id, signer, _, err := NewSigner()
	require.NoError(t, err)

	des := &Deserializer{}
	verifier, err := des.DeserializeVerifier(id)
	require.NoError(t, err)

	msg := []byte("payload")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sig))
}

func TestDeserializer_DeserializeVerifier_Invalid(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	_, err := des.DeserializeVerifier([]byte("garbage"))
	require.Error(t, err)
}

func TestDeserializer_DeserializeSigner(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	_, err := des.DeserializeSigner(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")
}

func TestDeserializer_Info_WithTestdata(t *testing.T) {
	t.Parallel()
	certPEM, err := os.ReadFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)

	raw := serializeIdentity(t, "apple", certPEM)
	des := &Deserializer{}
	info, err := des.Info(raw, nil)
	require.NoError(t, err)
	require.Contains(t, info, "MSP.x509:")
	require.Contains(t, info, "apple")
	require.Contains(t, info, "auditor.org1.example.com")
}

func TestDeserializer_Info_Invalid(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	_, err := des.Info([]byte("garbage"), nil)
	require.Error(t, err)
}

func TestDeserializer_String(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	require.Equal(t, "Generic X509 Verifier Deserializer", des.String())
}

func TestDeserializer_DeserializeVerifier_BadKey(t *testing.T) {
	t.Parallel()
	// SerializedIdentity with non-ECDSA PEM
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: badKey}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.DeserializeVerifier(raw)
	require.Error(t, err)
}

func TestDeserializer_Info_BadCertPEM(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: badCert}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.Info(raw, nil)
	require.Error(t, err)
}

func TestDeserializer_DeserializeVerifier_NonECDSA(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubPEM, err := PemEncodeKey(&rsaKey.PublicKey)
	require.NoError(t, err)

	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: pubPEM}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.DeserializeVerifier(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *ecdsa.PublicKey")
}

func TestDeserializer_Info_NonPEMIdBytes(t *testing.T) {
	t.Parallel()
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: []byte("not pem data")}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.Info(raw, nil)
	require.Error(t, err)
}
