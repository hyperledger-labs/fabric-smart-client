/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/stretchr/testify/assert"
)

func TestDeps(t *testing.T) {
	sp := view.NewServiceProvider()

	comm := &mock.CommLayer{}
	sp.RegisterService(comm)
	assert.Equal(t, comm, view.GetCommLayer(sp))

	es := &mock.EndpointService{}
	sp.RegisterService(es)
	assert.Equal(t, es, view.GetEndpointService(sp))

	ip := &mock.IdentityProvider{}
	sp.RegisterService(ip)
	assert.Equal(t, ip, view.GetIdentityProvider(sp))

	lic := &mock.LocalIdentityChecker{}
	sp.RegisterService(lic)
	assert.Equal(t, lic, view.GetLocalIdentityChecker(sp))
}
