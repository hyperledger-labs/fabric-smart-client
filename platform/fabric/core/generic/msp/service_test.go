/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"os"
	"testing"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	msp2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/config_provider.go -fake-name ConfigProvider . ConfigProvider

func TestRegisterIdemixLocalMSP(t *testing.T) {
	registry := registry2.New()

	cp := &mock2.ConfigProvider{}
	cp.IsSetReturns(false)
	assert.NoError(t, registry.RegisterService(cp))
	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	des, err := sig.NewMultiplexDeserializer(registry)
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(des))
	config, err := config2.New(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(registry, config, nil, nil, nil, 100)
	assert.NoError(t, registry.RegisterService(mspService))
	sigService := sig.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	assert.NoError(t, mspService.RegisterIdemixMSP("apple", "./idemix/testdata/idemix", "idemix"))
	ii := mspService.GetIdentityInfoByLabel(msp2.IdemixMSP, "apple")
	assert.NotNil(t, ii)
	assert.Equal(t, "apple", ii.ID)
	assert.Equal(t, "idemix", ii.EnrollmentID)
	id, info, err := ii.GetIdentity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Nil(t, info)
}

func TestIdemixTypeFolder(t *testing.T) {
	registry := registry2.New()

	cp, err := config.NewProvider("./testdata/idemixtypefolder")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(cp))
	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	des, err := sig.NewMultiplexDeserializer(registry)
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(des))
	config, err := config2.New(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(registry, config, nil, nil, nil, 100)
	assert.NoError(t, registry.RegisterService(mspService))
	sigService := sig.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	assert.NoError(t, mspService.Load())
	assert.Equal(t, []string{"idemix", "manager.id1", "manager.id2", "manager.id3", "apple"}, mspService.Msps())

	for _, s := range mspService.Msps()[:4] {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel("idemix", s))
	}
}

func TestRegisterX509LocalMSP(t *testing.T) {
	registry := registry2.New()

	cp := &mock2.ConfigProvider{}
	cp.IsSetReturns(false)
	assert.NoError(t, registry.RegisterService(cp))
	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	des, err := sig.NewMultiplexDeserializer(registry)
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(des))
	config, err := config2.New(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(registry, config, nil, nil, nil, 100)
	assert.NoError(t, registry.RegisterService(mspService))
	sigService := sig.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	assert.NoError(t, mspService.RegisterX509MSP("apple", "./x509/testdata/msp", "x509"))
	ii := mspService.GetIdentityInfoByLabel(msp2.BccspMSP, "apple")
	assert.NotNil(t, ii)
	assert.Equal(t, "apple", ii.ID)
	assert.Equal(t, "auditor.org1.example.com", ii.EnrollmentID)
	id, info, err := ii.GetIdentity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.NotNil(t, info)
}

func TestX509TypeFolder(t *testing.T) {
	registry := registry2.New()

	cp, err := config.NewProvider("./testdata/x509typefolder")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(cp))
	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	des, err := sig.NewMultiplexDeserializer(registry)
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(des))
	config, err := config2.New(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(registry, config, nil, nil, nil, 100)
	assert.NoError(t, registry.RegisterService(mspService))
	sigService := sig.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	assert.NoError(t, mspService.Load())
	assert.Equal(t, []string{"Admin@org1.example.com", "auditor@org1.example.com", "issuer.id1@org1.example.com"}, mspService.Msps())

	for _, s := range mspService.Msps() {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel(msp2.BccspMSP, s))
	}
}

func TestRefresh(t *testing.T) {
	registry := registry2.New()

	cp, err := config.NewProvider("./testdata/x509typefolder")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(cp))
	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	des, err := sig.NewMultiplexDeserializer(registry)
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(des))
	config, err := config2.New(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(registry, config, nil, nil, nil, 100)
	assert.NoError(t, registry.RegisterService(mspService))
	sigService := sig.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	assert.NoError(t, mspService.Load())
	assert.Equal(t, []string{"Admin@org1.example.com", "auditor@org1.example.com", "issuer.id1@org1.example.com"}, mspService.Msps())

	for _, s := range mspService.Msps() {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel(msp2.BccspMSP, s))
	}

	// copy new identity and refresh
	assert.NoError(t, copy.Copy("./testdata/manager@org2.example.com", "./testdata/x509typefolder/msps/manager@org2.example.com"))

	assert.NoError(t, mspService.Refresh())
	assert.Equal(t, []string{
		"Admin@org1.example.com",
		"auditor@org1.example.com",
		"issuer.id1@org1.example.com",
		"manager@org2.example.com",
	}, mspService.Msps())

	for _, s := range mspService.Msps() {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel(msp2.BccspMSP, s))
	}

	assert.NoError(t, os.RemoveAll("./testdata/x509typefolder/msps/manager@org2.example.com"))

	assert.NoError(t, mspService.Refresh())
	assert.Equal(t, []string{
		"Admin@org1.example.com",
		"auditor@org1.example.com",
		"issuer.id1@org1.example.com",
	}, mspService.Msps())

	for _, s := range mspService.Msps() {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel(msp2.BccspMSP, s))
	}
}
