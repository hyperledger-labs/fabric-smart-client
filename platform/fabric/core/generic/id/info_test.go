/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	x5092 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	msp2 "github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

func newAuditInfo() driver.AuditInfoPersistence {
	return utils.MustGet((&mem.Driver{}).NewAuditInfo("", nil))
}

func newSignerInfo() driver.SignerInfoPersistence {
	return utils.MustGet((&mem.Driver{}).NewSignerInfo("", nil))
}

func TestInfoIdemix(t *testing.T) {
	registry := registry2.New()

	kvss, err := kvs.NewWithConfig(&mem.Driver{}, "", &mock.ConfigProvider{})
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(), newSignerInfo())
	assert.NoError(t, registry.RegisterService(sigService))

	config, err := msp2.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	assert.NoError(t, err)

	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	id, _, err := p.Identity(nil)
	assert.NoError(t, err)

	s := Info(id, nil)
	assert.True(t, strings.HasPrefix(s, "MSP.Idemix: []"))
	assert.True(t, strings.HasSuffix(s, "[idemix][idemixorg.example.com][ADMIN]"))
}

func TestInfoX509(t *testing.T) {
	p, err := x5092.NewProvider("./testdata/x509", "", "apple", nil)
	assert.NoError(t, err)
	id, _, err := p.Identity(nil)
	assert.NoError(t, err)

	s := Info(id, nil)
	assert.Equal(t, "MSP.x509: [f+hVlmGaPejN2G0XDcESSMX2ol29WPcPQ+Fp3lOARBQ=][apple][auditor.org1.example.com]", s)
}
