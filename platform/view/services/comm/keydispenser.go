/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"reflect"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/libp2p/go-libp2p-core/crypto"
)

type PrivateKeyDispenser interface {
	PrivateKey() (crypto.PrivKey, error)
}

func GetPrivateKeyDispenser(sp view2.ServiceProvider) PrivateKeyDispenser {
	s, err := sp.GetService(reflect.TypeOf((*PrivateKeyDispenser)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(PrivateKeyDispenser)
}
