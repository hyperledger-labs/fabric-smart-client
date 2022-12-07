/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id/mock"
)

func TestLoad(t *testing.T) {
	cp := &mock.ConfigProvider{}
	cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/default.pem")
	cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
	cp.GetStringSliceReturnsOnCall(0, []string{
		"./testdata/client/client.pem",
	})
	cp.TranslatePathReturnsOnCall(0, "./testdata/client/client.pem")
	sigService := &mock.SigService{}

	idProvider := id.NewProvider(cp, sigService, nil)
	assert.NoError(t, idProvider.Load(), "failed loading identities")

	raw, err := id.LoadIdentity("./testdata/default/signcerts/default.pem")
	assert.NoError(t, err)
	assert.Equal(t, raw, []byte(idProvider.DefaultIdentity()))

	raw, err = id.LoadIdentity("./testdata/client/client.pem")
	assert.NoError(t, err)
	assert.Len(t, idProvider.Clients(), 1)
	assert.Equal(t, raw, []byte(idProvider.Clients()[0]))
}
