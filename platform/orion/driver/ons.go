/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

// OrionNetworkService gives access to a Orion network components
type OrionNetworkService interface {
	Name() string
}

type OrionNetworkServiceProvider interface {
	Names() []string
	DefaultName() string
	// OrionNetworkService returns a OrionNetworkService instance for the passed parameters
	OrionNetworkService(id string) (OrionNetworkService, error)
}

func GetOrionNetworkServiceProvider(ctx view2.ServiceProvider) OrionNetworkServiceProvider {
	s, err := ctx.GetService(reflect.TypeOf((*OrionNetworkServiceProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(OrionNetworkServiceProvider)
}
