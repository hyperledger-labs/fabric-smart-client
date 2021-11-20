/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

func ComputeTxID(id *driver.TxID) string {
	if len(id.Nonce) == 0 {
		n, err := GetRandomNonce()
		if err != nil {
			panic(err)
		}
		id.Nonce = n
	}
	return ComputeTxIDFull(id.Nonce, id.Creator)
}

func ComputeTxIDFull(nonce, creator []byte) string {
	hasher := sha256.New()
	hasher.Write(nonce)
	hasher.Write(creator)
	return hex.EncodeToString(hasher.Sum(nil))
}

func GetRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}
