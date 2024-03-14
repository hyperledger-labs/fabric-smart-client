/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
)

func newCryptoPrivKeyFromMSP(secretKeyPath string) (crypto.PrivKey, error) {
	fileCont, err := os.ReadFile(secretKeyPath)
	if err != nil {
		return nil, err
	}
	if len(fileCont) == 0 {
		return nil, errors.New("invalid pem, it must be different from nil")
	}
	block, _ := pem.Decode(fileCont)
	if block == nil {
		return nil, errors.Errorf("failed decoding pem, block must be different from nil [% x]", fileCont)
	}

	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	priv, _, err := crypto.ECDSAKeyPairFromKey(k.(*ecdsa.PrivateKey))
	if err != nil {
		return nil, err
	}

	return priv, nil
}
