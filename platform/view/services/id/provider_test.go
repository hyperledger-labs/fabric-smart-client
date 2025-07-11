/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id_test

import (
	"testing"

	id2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver/file"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/mock"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	cp := &mock2.ConfigProvider{}
	cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/default.pem")
	cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
	cp.GetStringSliceReturnsOnCall(0, []string{
		"./testdata/client/client.pem",
	})
	cp.TranslatePathReturnsOnCall(0, "./testdata/client/client.pem")
	sigService := &mock2.SigService{}

	idProvider, err := id2.NewProvider(cp, sigService, nil, &kms.KMS{Driver: &file.Driver{}})
	assert.NoError(t, err, "failed loading identities")

	raw, err := id2.LoadIdentity("./testdata/default/signcerts/default.pem")
	assert.NoError(t, err)
	assert.Equal(t, raw, []byte(idProvider.DefaultIdentity()))

	raw, err = id2.LoadIdentity("./testdata/client/client.pem")
	assert.NoError(t, err)
	assert.Len(t, idProvider.Clients(), 1)
	assert.Equal(t, raw, []byte(idProvider.Clients()[0]))
}
