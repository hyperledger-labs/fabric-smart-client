/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"crypto/x509"
	"encoding/pem"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// LoadIdentity reads the PEM-encoded certificate at certPath and returns it in
// the canonical form CanonicalIdentity produces. It fails if the file holds no
// parsable certificate.
func LoadIdentity(certPath string) ([]byte, error) {
	raw, err := readPemFile(certPath)
	if err != nil {
		return nil, err
	}
	return canonicalCert(raw)
}

// CanonicalIdentity returns raw with its first PEM block re-encoded as a bare
// CERTIFICATE block of the parsed certificate's DER bytes, dropping PEM
// headers, surrounding text, further blocks and whitespace differences. This
// is the form LoadIdentity returns, so two encodings of the same certificate
// map to the same bytes. raw is returned unchanged if its first PEM block is
// not a parsable certificate.
func CanonicalIdentity(raw []byte) []byte {
	out, err := canonicalCert(raw)
	if err != nil {
		return raw
	}
	return out
}

func canonicalCert(raw []byte) ([]byte, error) {
	cert, err := getCertFromPem(raw)
	if err != nil {
		return nil, err
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Bytes: cert.Raw, Type: "CERTIFICATE"})
	if pemBytes == nil {
		return nil, errors.New("encoding of identity failed")
	}

	return pemBytes, nil
}

func readFile(file string) ([]byte, error) {
	fileCont, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read file %s", file)
	}

	return fileCont, nil
}

func readPemFile(file string) ([]byte, error) {
	bytes, err := readFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "reading from file %s failed", file)
	}

	b, _ := pem.Decode(bytes)
	if b == nil { // TODO: also check that the type is what we expect (cert vs key..)
		return nil, errors.Errorf("no pem content for file %s", file)
	}

	return bytes, nil
}

func getCertFromPem(idBytes []byte) (*x509.Certificate, error) {
	if idBytes == nil {
		return nil, errors.New("getCertFromPem error: nil idBytes")
	}

	// Decode the pem bytes
	pemCert, _ := pem.Decode(idBytes)
	if pemCert == nil {
		return nil, errors.Errorf("getCertFromPem error: could not decode pem bytes [%v]", idBytes)
	}

	// get a cert
	var cert *x509.Certificate
	cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "getCertFromPem error: failed to parse x509 cert")
	}

	return cert, nil
}
