/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"hash"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Hasher interface {
	GetHash() hash.Hash

	Hash(msg []byte) ([]byte, error)
}

func GetHasher(sp view.ServiceProvider) Hasher {
	s, err := sp.GetService(reflect.TypeOf((*Hasher)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(Hasher)
}
