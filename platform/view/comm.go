/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// CommService provides endpoint-related services
type CommService struct {
	es driver.CommService
}

func (c *CommService) Addresses(id view.Identity) ([]string, error) {
	return c.es.Addresses(id)
}

// GetCommService returns an instance of the endpoint service.
// It panics, if no instance is found.
func GetCommService(sp ServiceProvider) *CommService {
	return &CommService{es: driver.GetCommService(sp)}
}
