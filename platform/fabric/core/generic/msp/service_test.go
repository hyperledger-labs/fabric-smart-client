/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"context"
	"errors"
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

const (
	testCacheSize = 100
)

func TestRegisterIdemixLocalMSP(t *testing.T) { //nolint:paralleltest
	cp := &mock.ConfigProvider{}
	cp.IsSetReturns(false)

	kvss, err := kvs.New(utils.MustGet(mem.NewDriver().NewKVS("")), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	des := sig.NewMultiplexDeserializer()

	config, err := config2.NewService(cp, "default", true)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, testCacheSize)

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
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, testCacheSize)

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
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, testCacheSize)

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
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, testCacheSize)

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
	mspService := msp.NewLocalMSPManager(config, kvss, nil, nil, nil, des, testCacheSize)

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

// mspManager is a test interface that defines the methods needed for testing the MSP
// service.
// It allows us to test the service's exported behavior without depending on internal
// implementation details.
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

// setup creates a test fixture with a configured MSP service and its dependencies.
// It initializes:
// - A mock ConfigProvider configured to return "default_msp" as the default MSP
// - An in-memory KVS for storage
// - Mock SignerService and BinderService for testing identity operations
// - A configured MSP service ready for testing
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

	mspService := msp.NewLocalMSPManager(config, kvss, signerService, binderService, defaultViewIdentity, des, testCacheSize)

	return mspService, signerService, binderService, config
}

func TestService_DefaultAccessors(t *testing.T) {
	t.Parallel()
	mspService, signerService, _, config := setup(t)
	require.Equal(t, "default_msp", mspService.DefaultMSP())
	require.Equal(t, signerService, mspService.SignerService())
	require.Equal(t, testCacheSize, mspService.CacheSize())
	require.Equal(t, config, mspService.Config())
}

func TestService_SetDefaultIdentity(t *testing.T) {
	t.Parallel()
	t.Run("MatchingDefaultMSP", func(t *testing.T) {
		t.Parallel()
		mspService, _, _, _ := setup(t)

		// Load() is needed to initialize the internal defaultMSP field
		// We expect it to fail since no MSPs are configured, but it will set defaultMSP
		_ = mspService.Load()

		id := view.Identity("id1")
		sid := &mock.SigningIdentity{}
		mspService.SetDefaultIdentity("default_msp", id, sid)
		require.Equal(t, id, mspService.DefaultIdentity())
		require.Equal(t, sid, mspService.DefaultSigningIdentity())
	})

	t.Run("NonMatchingID", func(t *testing.T) {
		t.Parallel()
		mspService, _, _, _ := setup(t)

		// Load() is needed to initialize the internal defaultMSP field
		_ = mspService.Load()

		// Set initial identity
		initialId := view.Identity("initial_id")
		initialSid := &mock.SigningIdentity{}
		mspService.SetDefaultIdentity("default_msp", initialId, initialSid)

		// Try to set with non-matching id - should be no-op
		newId := view.Identity("new_id")
		newSid := &mock.SigningIdentity{}
		mspService.SetDefaultIdentity("non_matching_msp", newId, newSid)

		// Verify identity remains unchanged
		require.Equal(t, initialId, mspService.DefaultIdentity())
		require.Equal(t, initialSid, mspService.DefaultSigningIdentity())
	})
}

func TestService_IsMe(t *testing.T) {
	t.Parallel()
	id := view.Identity("id1")

	t.Run("True", func(t *testing.T) {
		t.Parallel()
		mspService, signerService, _, _ := setup(t)
		signerService.IsMeReturns(true)
		require.True(t, mspService.IsMe(t.Context(), id))
	})

	t.Run("False", func(t *testing.T) {
		t.Parallel()
		mspService, signerService, _, _ := setup(t)
		signerService.IsMeReturns(false)
		require.False(t, mspService.IsMe(t.Context(), id))
	})
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

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		mspService, _, binderService, _ := setup(t)
		binderService.GetIdentityReturns(nil, errors.New("identity not found"))
		_, err := mspService.GetIdentityByID("non_existent")
		require.Error(t, err)
		require.ErrorContains(t, err, "identity [non_existent] not found")
	})
}

func TestService_Identity(t *testing.T) {
	t.Parallel()
	mspService, _, _, _ := setup(t)
	id := view.Identity("id1")
	require.NoError(t, mspService.AddMSP("apple", msp.BccspMSP, "enrollment1", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return id, nil, nil
	}))

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		resId, err := mspService.Identity("apple")
		require.NoError(t, err)
		require.Equal(t, id, resId)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		resId, err := mspService.Identity("unknown")
		require.Error(t, err)
		require.Nil(t, resId)
	})
}

func TestService_GetIdentityInfoByIdentity_BCCSP(t *testing.T) {
	t.Parallel()
	mspService, _, _, _ := setup(t)
	id := view.Identity("id1")

	require.NoError(t, mspService.AddMSP("apple", msp.BccspMSP, "enrollment1", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return id, nil, nil
	}))

	t.Run("MatchingDefaultMSP", func(t *testing.T) {
		t.Parallel()
		info := mspService.GetIdentityInfoByIdentity(msp.BccspMSP, id)
		require.NotNil(t, info)
		require.Equal(t, "apple", info.ID)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, mspService.GetIdentityInfoByIdentity(msp.BccspMSP, view.Identity("unknown")))
	})
}

func TestService_GetIdentityInfoByIdentity_Idemix(t *testing.T) {
	t.Parallel()
	mspService, _, _, _ := setup(t)
	idemixId := view.Identity("idemix_id")
	require.NoError(t, mspService.AddMSP("orange", msp.IdemixMSP, "enrollment2", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
		return idemixId, nil, nil
	}))

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		info := mspService.GetIdentityInfoByIdentity(msp.IdemixMSP, idemixId)
		require.NotNil(t, info)
		require.Equal(t, "orange", info.ID)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, mspService.GetIdentityInfoByIdentity(msp.IdemixMSP, view.Identity("unknown")))
	})
}

func TestService_AnonymousIdentity(t *testing.T) {
	t.Parallel()

	t.Run("BindsDefaultViewIdentity", func(t *testing.T) {
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
	})

	t.Run("IdentityNotFound", func(t *testing.T) {
		t.Parallel()
		mspService, _, _, _ := setup(t)
		anonId, err := mspService.AnonymousIdentity()
		require.Error(t, err)
		require.Nil(t, anonId)
	})

	t.Run("BindFails", func(t *testing.T) {
		t.Parallel()
		mspService, _, binderService, _ := setup(t)
		idemixId := view.Identity("idemix_id")
		binderService.BindReturns(errors.New("bind failed"))

		require.NoError(t, mspService.AddMSP("idemix", msp.IdemixMSP, "idemix_enrollment", func(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
			return idemixId, nil, nil
		}))

		anonId, err := mspService.AnonymousIdentity()
		require.Error(t, err)
		require.Nil(t, anonId)
	})
}
