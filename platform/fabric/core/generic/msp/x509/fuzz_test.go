/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzDeserializeVerifier fuzzes Deserializer.DeserializeVerifier and Deserializer.Info
// with arbitrary wire bytes. NewIdentityFromBytes parses identities through the exact
// same proto.Unmarshal -> PemDecodeKey -> *ecdsa.PublicKey path as DeserializeVerifier,
// so a single target covers both without a duplicated seed corpus.
func FuzzDeserializeVerifier(f *testing.F) {
	validID, _, _, err := NewSigner()
	require.NoError(f, err)
	f.Add([]byte(validID))

	_, certPEM := generateSelfSignedCert(f)
	validCertID := serializeIdentity(f, "testmsp", certPEM)
	f.Add(validCertID)

	certPath := filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	if fileID, err := Serialize("apple", certPath); err == nil {
		f.Add(fileID)
	}

	f.Add(serializeIdentity(f, "testmsp", []byte("not a pem encoded cert")))
	f.Add(serializeIdentity(f, "testmsp", []byte("")))
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not a protobuf message"))
	f.Add([]byte{0x0a, 0x04, 0x74, 0x65, 0x73, 0x74}) // protobuf mspid="test" without IdBytes

	d := &Deserializer{}
	f.Fuzz(func(_ *testing.T, raw []byte) {
		_, _ = d.DeserializeVerifier(raw)
		_, _ = d.Info(raw, nil)
	})
}
