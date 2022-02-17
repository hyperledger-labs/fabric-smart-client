/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

var metricsType = reflect.TypeOf((*Metrics)(nil))

type Metrics interface {
	EmitKey(val float32, event ...string)
}

func Get(sp view.ServiceProvider) Metrics {
	s, err := sp.GetService(metricsType)
	if err != nil {
		panic(err)
	}
	return s.(Metrics)
}
