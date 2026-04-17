//go:build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/ecdsa"
	"errors"
)

const Provider = "PKCS11"

// FindPKCS11Lib is a stub that returns an error when pkcs11 support is not compiled in.
func FindPKCS11Lib() (lib, pin, label string, err error) {
	err = errors.New("pkcs11 not included in build; rebuild with: go build -tags pkcs11")
	return lib, pin, label, err
}

// GeneratePrivateKey is a stub that returns an error when pkcs11 support is not compiled in.
func GeneratePrivateKey() (*ecdsa.PublicKey, error) {
	return nil, errors.New("pkcs11 not included in build; rebuild with: go build -tags pkcs11")
}
