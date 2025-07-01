/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

type Stream interface {
	Recv(m any) error
	Send(m any) error
}

func GetStream(sp services.Provider) Stream {
	scsBoxed, err := GetStreamIfExists(sp)
	if err != nil {
		panic(err)
	}
	return scsBoxed
}

func GetStreamIfExists(sp services.Provider) (Stream, error) {
	scsBoxed, err := sp.GetService(reflect.TypeOf((*Stream)(nil)))
	if err != nil {
		return nil, err
	}
	return scsBoxed.(Stream), nil
}
