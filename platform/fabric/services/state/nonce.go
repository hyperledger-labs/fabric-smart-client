/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"crypto/rand"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// CreateNonceOrPanic generates a nonce using the common/crypto package
// and panics if this operation fails.
func CreateNonceOrPanic() []byte {
	nonce, err := CreateNonce()
	if err != nil {
		panic(err)
	}
	return nonce
}

// CreateNonce generates a nonce using the common/crypto package.
func CreateNonce() ([]byte, error) {
	nonce, err := getRandomNonce()
	return nonce, errors.WithMessage(err, "error generating random nonce")
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}
