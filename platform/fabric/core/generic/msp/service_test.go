/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

type mspManager interface {
	DefaultMSP() string
	SetDefaultIdentity(id string, defaultIdentity view.Identity, defaultSigningIdentity driver.SigningIdentity)
	DefaultIdentity() view.Identity
	DefaultSigningIdentity() fdriver.SigningIdentity
	SignerService() driver.SignerService
	CacheSize() int
	Config() driver.Config
	IsMe(ctx context.Context, id view.Identity) bool
	AddMSP(name, mspType, enrollmentID string, IdentityGetter fdriver.GetIdentityFunc) error
	GetIdentityByID(id string) (view.Identity, error)
	Identity(label string) (view.Identity, error)
	GetIdentityInfoByIdentity(mspType string, id view.Identity) *fdriver.IdentityInfo
	AnonymousIdentity() (view.Identity, error)
	Load() error
}

func setup(t *testing.T) (mspManager, *mock.SignerService, *mock.BinderService, driver.Config) {
	t.Helper()
	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)
	cp.GetStringReturns("default_msp")

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	des := sig.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)

	signerService := &mock.SignerService{}
	binderService := &mock.BinderService{}
	defaultViewIdentity := view.Identity("default_view_identity")

	mspService := msp.NewLocalMSPManager(config, kvss, signerService, binderService, defaultViewIdentity, des, 100)

	// In order to set defaultMSP field via exported Load() method,
	// we need to make sure config.MSPs() returns something (even if empty)
	// and config.DefaultMSP() returns what we want.
	// But Wait! config2.Service uses the ConfigProvider we passed.
	cp.GetStringReturns("default_msp") // for DefaultMSP()

	// We can't easily mock config.MSPs() because it's part of config2.Service which uses ConfigProvider.
	// However, we don't need to call Load() if we can just test the exported behaviors.

	return mspService, signerService, binderService, config
}

func TestService_DefaultAccessors(t *testing.T) {
	t.Parallel()
	mspService, signerService, _, config := setup(t)
	require.Equal(t, "default_msp", mspService.DefaultMSP())
	require.Equal(t, signerService, mspService.SignerService())
	require.Equal(t, 100, mspService.CacheSize())
	require.Equal(t, config, mspService.Config())
}

func TestService_SetDefaultIdentity(t *testing.T) {
	t.Parallel()
	// NOTE: Since defaultMSP is unexported and only set via Load(),
	// and Load() requires a complex setup, we might need a workaround.
	// But wait! If we are in msp_test, we can't set it.
	// Let's see if Load() can be made to work.

	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)
	cp.GetStringReturns("default_msp")

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	des := sig.NewMultiplexDeserializer()
	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	signerService := &mock.SignerService{}
	binderService := &mock.BinderService{}

	mspService := msp.NewLocalMSPManager(config, kvss, signerService, binderService, view.Identity("default"), des, 100)

	// We need Load() to set s.defaultMSP.
	// loadLocalMSPs() calls s.config.MSPs().
	// config2.Service.MSPs() reads from ConfigProvider.
	// It looks for "msps" slice.
	cp.IsSetReturns(true)
	// If we don't want to provide a real slice, we can just let it be empty.

	// Actually, there is a simpler way. The maintainer wants msp_test.
	// If the test needs to set an unexported field, it's a sign that:
	// a) The field should be set via an exported method (like Load).
	// b) The test belongs in the internal package.
	// But the maintainer specifically asked for msp_test.

	// I'll use a small trick: use Load() and mock the config enough.
	require.Error(t, mspService.Load()) // It will fail because no default identity, but it will set s.defaultMSP

	id := view.Identity("id1")
	sid := &mock.SigningIdentity{}
	mspService.SetDefaultIdentity("default_msp", id, sid)
	require.Equal(t, id, mspService.DefaultIdentity())
	require.Equal(t, sid, mspService.DefaultSigningIdentity())
}

func TestService_IsMe(t *testing.T) {
	t.Parallel()
	mspService, signerService, _, _ := setup(t)
	id := view.Identity("id1")
	signerService.IsMeReturns(true)
	require.True(t, mspService.IsMe(context.Background(), id))
	signerService.IsMeReturns(false)
	require.False(t, mspService.IsMe(context.Background(), id))
}

func TestService_GetIdentityByID(t *testing.T) {
	t.Parallel()
	mspService, _, binderService, _ := setup(t)
	id := view.Identity("id1")

	require.NoError(t, mspService.AddMSP("apple", msp.BccspMSP, "enrollment1", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return id, nil, nil
	}))

	t.Run("ByMSPName", func(t *testing.T) {
		t.Parallel()
		resId, err := mspService.GetIdentityByID("apple")
		require.NoError(t, err)
		require.Equal(t, id, resId)
	})

	t.Run("ByEnrollmentID", func(t *testing.T) {
		t.Parallel()
		resId, err := mspService.GetIdentityByID("enrollment1")
		require.NoError(t, err)
		require.Equal(t, id, resId)
	})

	t.Run("ViaBinderService", func(t *testing.T) {
		t.Parallel()
		binderId := view.Identity("binder_id")
		binderService.GetIdentityReturns(binderId, nil)
		resId, err := mspService.GetIdentityByID("non_existent")
		require.NoError(t, err)
		require.Equal(t, binderId, resId)
	})
}

func TestService_Identity(t *testing.T) {
	t.Parallel()
	mspService, _, _, _ := setup(t)
	id := view.Identity("id1")
	require.NoError(t, mspService.AddMSP("apple", msp.BccspMSP, "enrollment1", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return id, nil, nil
	}))

	resId, err := mspService.Identity("apple")
	require.NoError(t, err)
	require.Equal(t, id, resId)
}

func TestService_GetIdentityInfoByIdentity_BCCSP(t *testing.T) {
	t.Parallel()
	mspService, _, _, _ := setup(t)
	id := view.Identity("id1")

	require.NoError(t, mspService.AddMSP("apple", msp.BccspMSP, "enrollment1", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return id, nil, nil
	}))

	info := mspService.GetIdentityInfoByIdentity(msp.BccspMSP, id)
	require.NotNil(t, info)
	require.Equal(t, "apple", info.ID)

	require.Nil(t, mspService.GetIdentityInfoByIdentity(msp.BccspMSP, view.Identity("unknown")))
}

func TestService_GetIdentityInfoByIdentity_Idemix(t *testing.T) {
	t.Parallel()
	mspService, _, _, _ := setup(t)
	idemixId := view.Identity("idemix_id")
	require.NoError(t, mspService.AddMSP("orange", msp.IdemixMSP, "enrollment2", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return idemixId, nil, nil
	}))

	info := mspService.GetIdentityInfoByIdentity(msp.IdemixMSP, idemixId)
	require.NotNil(t, info)
	require.Equal(t, "orange", info.ID)
}

func TestService_AnonymousIdentity_BindsDefaultViewIdentity(t *testing.T) {
	t.Parallel()
	mspService, _, binderService, _ := setup(t)
	idemixId := view.Identity("idemix_id")

	initialCount := binderService.BindCallCount()

	require.NoError(t, mspService.AddMSP("idemix", msp.IdemixMSP, "idemix_enrollment", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return idemixId, nil, nil
	}))

	anonId, err := mspService.AnonymousIdentity()
	require.NoError(t, err)
	require.Equal(t, idemixId, anonId)
	require.Equal(t, initialCount+1, binderService.BindCallCount())
}
