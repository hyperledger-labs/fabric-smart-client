/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

var key = reflect.TypeOf((*Metrics)(nil))

type Metrics interface {
	EmitKey(val float32, event ...string)
}

func Get(sp view.ServiceProvider) Metrics {
	s, err := sp.GetService(key)
	if err != nil {
		panic(err)
	}
	return s.(Metrics)
}
