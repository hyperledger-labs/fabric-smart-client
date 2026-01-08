/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"crypto/rand"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
)

func ComputeTxID(id *driver.TxIDComponents) string {
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
