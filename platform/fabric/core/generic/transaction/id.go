/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package transaction

import (
	"crypto/rand"

	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
)

func ComputeTxID(id *api.TxID) string {
	if len(id.Nonce) == 0 {
		n, err := GetRandomNonce()
		if err != nil {
			panic(err)
		}
		id.Nonce = n
	}
	return protoutil.ComputeTxID(id.Nonce, id.Creator)
}

func GetRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}
