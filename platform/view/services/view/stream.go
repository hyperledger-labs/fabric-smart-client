/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Stream interface {
	Recv(m interface{}) error
	Send(m interface{}) error
}

func GetStream(sp view.ServiceProvider) Stream {
	scsBoxed, err := sp.GetService(reflect.TypeOf((*Stream)(nil)))
	if err != nil {
		panic(err)
	}
	return scsBoxed.(Stream)
}
