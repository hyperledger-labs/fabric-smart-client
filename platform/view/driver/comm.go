/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var (
	CommServiceID = reflect.TypeOf((*CommService)(nil))
)

type CommService interface {
	Addresses(id view.Identity) ([]string, error)
}

// GetCommService returns an instance of the communication service.
// It panics, if no instance is found.
func GetCommService(ctx ServiceProvider) CommService {
	s, err := ctx.GetService(CommServiceID)
	if err != nil {
		panic(err)
	}
	return s.(CommService)
}
