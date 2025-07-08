/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// This wrapper is needed in order to be able to fetch the provider using the SP from the Node
var providerType = reflect.TypeOf((*wrappedProvider)(nil))

type wrappedProvider struct {
	tracing.Provider
}

func NewWrappedProvider(tp tracing.Provider) tracing.Provider {
	return &wrappedProvider{Provider: tp}
}

func Get(sp services.Provider) tracing.Provider {
	s, err := sp.GetService(providerType)
	if err != nil {
		panic(err)
	}
	return s.(*wrappedProvider)
}
