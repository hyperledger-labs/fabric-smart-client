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
	err := sp.RegisterService(comm)
	assert.NoError(t, err)
	assert.Equal(t, comm, view.GetCommLayer(sp))

	es := &mock.EndpointService{}
	err = sp.RegisterService(es)
	assert.NoError(t, err)
	assert.Equal(t, es, view.GetEndpointService(sp))

	ip := &mock.IdentityProvider{}
	err = sp.RegisterService(ip)
	assert.NoError(t, err)
	assert.Equal(t, ip, view.GetIdentityProvider(sp))

	lic := &mock.LocalIdentityChecker{}
	err = sp.RegisterService(lic)
	assert.NoError(t, err)
	assert.Equal(t, lic, view.GetLocalIdentityChecker(sp))
}
