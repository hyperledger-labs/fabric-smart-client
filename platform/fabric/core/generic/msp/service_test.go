/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"os"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	msp2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/stretchr/testify/assert"
)

func TestRegisterIdemixLocalMSP(t *testing.T) {
	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	assert.NoError(t, err)

	des := sig2.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	assert.NoError(t, mspService.RegisterIdemixMSP("apple", "./idemix/testdata/idemix", "idemix"))
	ii := mspService.GetIdentityInfoByLabel(msp2.IdemixMSP, "apple")
	assert.NotNil(t, ii)
	assert.Equal(t, "apple", ii.ID)
	assert.Equal(t, "alice", ii.EnrollmentID)

	id, info, err := ii.GetIdentity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Nil(t, info)
}

func TestIdemixTypeFolder(t *testing.T) {
	cp, err := config.NewProvider("./testdata/idemixtypefolder")
	assert.NoError(t, err)
	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	assert.NoError(t, err)
	des := sig2.NewMultiplexDeserializer()
	config, err := config2.NewService(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	assert.NoError(t, mspService.Load())
	assert.Equal(t, []string{"idemix", "manager.id1", "manager.id2", "manager.id3", "apple"}, mspService.Msps())

	for _, s := range mspService.Msps()[:4] {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel("idemix", s))
	}
}

func TestRegisterX509LocalMSP(t *testing.T) {
	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	assert.NoError(t, err)

	des := sig2.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

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
	cp, err := config.NewProvider("./testdata/x509typefolder")
	assert.NoError(t, err)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	assert.NoError(t, err)

	des := sig2.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	assert.NoError(t, mspService.Load())
	assert.Equal(t, []string{"Admin@org1.example.com", "auditor@org1.example.com", "issuer.id1@org1.example.com"}, mspService.Msps())

	for _, s := range mspService.Msps() {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel(msp2.BccspMSP, s))
	}
}

func TestRefresh(t *testing.T) {
	cp, err := config.NewProvider("./testdata/x509typefolder")
	assert.NoError(t, err)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	assert.NoError(t, err)
	des := sig2.NewMultiplexDeserializer()
	config, err := config2.NewService(cp, "default", true)
	assert.NoError(t, err)
	mspService := msp2.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	assert.NoError(t, mspService.Load())
	assert.Equal(t, []string{"Admin@org1.example.com", "auditor@org1.example.com", "issuer.id1@org1.example.com"}, mspService.Msps())

	for _, s := range mspService.Msps() {
		assert.NotNil(t, mspService.GetIdentityInfoByLabel(msp2.BccspMSP, s))
	}

	// copy new identity and refresh
	assert.NoError(t, os.CopyFS("./testdata/x509typefolder/msps/manager@org2.example.com", os.DirFS("./testdata/manager@org2.example.com")))

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
