/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger/fabric/common/metrics"
	"reflect"
)

var key = reflect.TypeOf((*metrics.Provider)(nil))

// GetProvider returns the metrics provider registered in the service provider passed in.
func GetProvider(sp view.ServiceProvider) metrics.Provider {
	s, err := sp.GetService(key)
	if err != nil {
		panic(err)
	}
	return s.(metrics.Provider)
}
