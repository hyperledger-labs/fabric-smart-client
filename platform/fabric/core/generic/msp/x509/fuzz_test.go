/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"path/filepath"
	"testing"
)

// FuzzECDSAVerify fuzzes edsaVerifier.Verify with arbitrary signature bytes (sigma).
// This is the entry point where untrusted wire bytes from peer endorsements or
// transaction envelopes are decoded via asn1.Unmarshal before signature verification.
func FuzzECDSAVerify(f *testing.F) {
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		f.Fatalf("failed generating ecdsa key: %v", err)
	}
	verifier := NewVerifier(&sk.PublicKey)
	signer := &edsaSigner{sk: sk}

	msg := []byte("untrusted payload to verify against signature")
	validSig, err := signer.Sign(msg)
	if err != nil {
		f.Fatalf("failed generating valid signature: %v", err)
	}

	f.Add(validSig)
	if len(validSig) > 2 {
		f.Add(validSig[:len(validSig)/2])
	}
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not an asn1 signature"))
	f.Add([]byte{0x30, 0x00})                                           // empty SEQUENCE
	f.Add([]byte{0x30, 0x06, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00})       // SEQUENCE of two zero INTEGERS
	f.Add([]byte{0x30, 0x06, 0x02, 0x01, 0x7f, 0x02, 0x01, 0x7f})       // SEQUENCE of small positive INTEGERS
	f.Add([]byte{0x30, 0x08, 0x02, 0x02, 0xff, 0xff, 0x02, 0x01, 0x01}) // SEQUENCE with negative R

	f.Fuzz(func(t *testing.T, sigma []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("edsaVerifier.Verify panicked on signature %q: %v", sigma, r)
			}
		}()
		_ = verifier.Verify(msg, sigma)
	})
}

// FuzzNewIdentityFromBytes fuzzes NewIdentityFromBytes with arbitrary serialized identity bytes.
// Untrusted identities arrive on the wire in proposals, envelopes, and session exchanges.
func FuzzNewIdentityFromBytes(f *testing.F) {
	validID, _, _, err := NewSigner()
	if err != nil {
		f.Fatalf("failed creating signer: %v", err)
	}
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

	f.Fuzz(func(t *testing.T, raw []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewIdentityFromBytes panicked on input %q: %v", raw, r)
			}
		}()
		_, _, _ = NewIdentityFromBytes(raw)
	})
}

// FuzzDeserializeVerifier fuzzes Deserializer.DeserializeVerifier and Deserializer.Info
// with arbitrary wire bytes.
func FuzzDeserializeVerifier(f *testing.F) {
	validID, _, _, err := NewSigner()
	if err != nil {
		f.Fatalf("failed creating signer: %v", err)
	}
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
	f.Add([]byte{0x0a, 0x04, 0x74, 0x65, 0x73, 0x74})

	f.Fuzz(func(t *testing.T, raw []byte) {
		stage := "DeserializeVerifier"
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Deserializer.%s panicked on input %q: %v", stage, raw, r)
			}
		}()
		d := &Deserializer{}
		_, _ = d.DeserializeVerifier(raw)
		stage = "Info"
		_, _ = d.Info(raw, nil)
	})
}
