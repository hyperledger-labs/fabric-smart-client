/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"strings"
	"testing"

	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	x5092 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/stretchr/testify/assert"
)

func TestInfoIdemix(t *testing.T) {
	driver := mem.NewDriver()
	persistence, err := driver.NewKVS("")
	assert.NoError(t, err)
	auditInfo, err := driver.NewAuditInfo("")
	assert.NoError(t, err)
	signerInfo, err := driver.NewSignerInfo("")
	assert.NoError(t, err)
	kvss, err := kvs.New(persistence, "", kvs.DefaultCacheSize)
	assert.NoError(t, err)
	sigService := sig2.NewService(sig2.NewMultiplexDeserializer(), auditInfo, signerInfo)

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
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
