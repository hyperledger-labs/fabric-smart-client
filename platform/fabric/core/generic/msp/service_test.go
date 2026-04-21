/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func TestRegisterIdemixLocalMSP(t *testing.T) { //nolint:paralleltest
	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	des := sig.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	require.NoError(t, mspService.RegisterIdemixMSP("apple", "./idemix/testdata/idemix", "idemix"))
	ii := mspService.GetIdentityInfoByLabel(msp.IdemixMSP, "apple")
	require.NotNil(t, ii)
	require.Equal(t, "apple", ii.ID)
	require.Equal(t, "alice", ii.EnrollmentID)

	id, info, err := ii.GetIdentity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, info)
}

func TestIdemixTypeFolder(t *testing.T) { //nolint:paralleltest
	cp, err := config.NewProvider("./testdata/idemixtypefolder")
	require.NoError(t, err)
	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	des := sig.NewMultiplexDeserializer()
	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	require.NoError(t, mspService.Load())
	require.Equal(t, []string{"idemix", "manager.id1", "manager.id2", "manager.id3", "apple"}, mspService.Msps())

	for _, s := range mspService.Msps()[:4] {
		require.NotNil(t, mspService.GetIdentityInfoByLabel("idemix", s))
	}
}

func TestRegisterX509LocalMSP(t *testing.T) { //nolint:paralleltest
	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	des := sig.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	require.NoError(t, mspService.RegisterX509MSP("apple", "./x509/testdata/msp", "x509"))
	ii := mspService.GetIdentityInfoByLabel(msp.BccspMSP, "apple")
	require.NotNil(t, ii)
	require.Equal(t, "apple", ii.ID)
	require.Equal(t, "auditor.org1.example.com", ii.EnrollmentID)
	id, info, err := ii.GetIdentity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, info)
}

func TestX509TypeFolder(t *testing.T) { //nolint:paralleltest
	cp, err := config.NewProvider("./testdata/x509typefolder")
	require.NoError(t, err)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	des := sig.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	require.NoError(t, mspService.Load())
	require.Equal(t, []string{"Admin@org1.example.com", "auditor@org1.example.com", "issuer.id1@org1.example.com"}, mspService.Msps())

	for _, s := range mspService.Msps() {
		require.NotNil(t, mspService.GetIdentityInfoByLabel(msp.BccspMSP, s))
	}
}

func TestRefresh(t *testing.T) {
	t.Parallel()
	cp, err := config.NewProvider("./testdata/x509typefolder")
	require.NoError(t, err)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	des := sig.NewMultiplexDeserializer()
	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, 100)

	require.NoError(t, mspService.Load())
	require.Equal(t, []string{"Admin@org1.example.com", "auditor@org1.example.com", "issuer.id1@org1.example.com"}, mspService.Msps())

	for _, s := range mspService.Msps() {
		require.NotNil(t, mspService.GetIdentityInfoByLabel(msp.BccspMSP, s))
	}

	// copy new identity and refresh
	require.NoError(t, os.CopyFS("./testdata/x509typefolder/msps/manager@org2.example.com", os.DirFS("./testdata/manager@org2.example.com")))

	require.NoError(t, mspService.Refresh())
	require.Equal(t, []string{
		"Admin@org1.example.com",
		"auditor@org1.example.com",
		"issuer.id1@org1.example.com",
		"manager@org2.example.com",
	}, mspService.Msps())

	for _, s := range mspService.Msps() {
		require.NotNil(t, mspService.GetIdentityInfoByLabel(msp.BccspMSP, s))
	}

	require.NoError(t, os.RemoveAll("./testdata/x509typefolder/msps/manager@org2.example.com"))

	require.NoError(t, mspService.Refresh())
	require.Equal(t, []string{
		"Admin@org1.example.com",
		"auditor@org1.example.com",
		"issuer.id1@org1.example.com",
	}, mspService.Msps())

	for _, s := range mspService.Msps() {
		require.NotNil(t, mspService.GetIdentityInfoByLabel(msp.BccspMSP, s))
	}
}
