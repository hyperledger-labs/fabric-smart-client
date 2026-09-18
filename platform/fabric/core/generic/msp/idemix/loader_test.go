/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func TestIdentityLoader_Load(t *testing.T) { //nolint:paralleltest
	for _, dir := range curveDirs(t) { //nolint:paralleltest
		t.Run(dir, func(t *testing.T) { //nolint:paralleltest
			kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
			require.NoError(t, err)

			sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

			loader := &idemix.IdentityLoader{
				KVS:           kvss,
				SignerService: sigService,
			}

			mockConfig := &mock.Config{}
			mockConfig.TranslatePathReturns(dir)

			mockManager := &mock.Manager{}
			mockManager.ConfigReturns(mockConfig)
			mockManager.CacheSizeReturns(10)

			mspConfig := config.MSP{
				ID:        "idemix",
				MSPType:   idemix.MSPType,
				MSPID:     "idemix",
				Path:      dir,
				CacheSize: 5,
				CurveID:   curveIDForDir(dir),
			}

			// Successful load
			err = loader.Load(mockManager, mspConfig)
			require.NoError(t, err)
			require.Equal(t, 1, mockManager.AddMSPCallCount())
			require.Equal(t, 1, mockManager.AddDeserializerCallCount())
			id, mspType, enrollmentID, idGetter := mockManager.AddMSPArgsForCall(0)
			require.Equal(t, "idemix", id)
			require.Equal(t, idemix.MSPType, mspType)
			require.NotEmpty(t, enrollmentID)
			require.NotNil(t, idGetter)

			// Error path: invalid config path
			mockConfigErr := &mock.Config{}
			mockConfigErr.TranslatePathReturns("./invalid/path")
			mockManagerErr := &mock.Manager{}
			mockManagerErr.ConfigReturns(mockConfigErr)

			err = loader.Load(mockManagerErr, mspConfig)
			require.ErrorContains(t, err, "failed reading idemix msp configuration")
		})
	}
}

func TestIdentityLoader_Load_NewProviderError(t *testing.T) { //nolint:paralleltest
	dir := curveDirs(t)[0]

	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.IdentityLoader{KVS: kvss, SignerService: sigService}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathReturns(dir)
	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)
	mockManager.CacheSizeReturns(10)

	mspConfig := config.MSP{
		ID:      "idemix",
		MSPType: idemix.MSPType,
		MSPID:   "idemix",
		Path:    dir,
		CurveID: "BOGUS_CURVE_ID_THAT_DOES_NOT_EXIST",
	}

	err = loader.Load(mockManager, mspConfig)
	require.ErrorContains(t, err, "failed instantiating idemix msp provider")
	require.Equal(t, 0, mockManager.AddMSPCallCount())
}

func TestIdentityLoader_Load_AddMSPError(t *testing.T) { //nolint:paralleltest
	dir := curveDirs(t)[0]

	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.IdentityLoader{KVS: kvss, SignerService: sigService}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathReturns(dir)
	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)
	mockManager.CacheSizeReturns(10)
	mockManager.AddMSPReturns(errors.New("add msp failure"))

	mspConfig := config.MSP{
		ID:      "idemix",
		MSPType: idemix.MSPType,
		MSPID:   "idemix",
		Path:    dir,
		CurveID: curveIDForDir(dir),
	}

	err = loader.Load(mockManager, mspConfig)
	require.ErrorContains(t, err, "failed adding idemix msp")
}

// copyFixtureMSP copies the msp/, user/ (and ca/, admin/ when present) folders from a
// curve fixture directory into dst, so FolderIdentityLoader tests can build a members
// folder without mutating the shared testdata/curves fixtures.
func copyFixtureMSP(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, exec.Command("cp", "-r", src+"/.", dst).Run()) //nolint:gosec
}

func TestFolderIdentityLoader_Load(t *testing.T) { //nolint:paralleltest
	// FolderIdentityLoader.Load never stamps a CurveID (unlike IdentityLoader.Load
	// driven directly from config.MSP.CurveID), so the fixture must already work
	// against the scheme's default curve; dlog/FP256BN_AMCL does.
	dir := filepath.Join(curvesRoot, "dlog", "FP256BN_AMCL")

	tmp := t.TempDir()
	copyFixtureMSP(t, dir, filepath.Join(tmp, "member1"))
	copyFixtureMSP(t, dir, filepath.Join(tmp, "member2"))
	// A plain file entry must be skipped rather than treated as an MSP folder.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "not-a-dir"), []byte("x"), 0o644))

	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.FolderIdentityLoader{IdentityLoader: &idemix.IdentityLoader{KVS: kvss, SignerService: sigService}}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathCalls(func(p string) string { return p })
	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)
	mockManager.CacheSizeReturns(10)

	err = loader.Load(mockManager, config.MSP{Path: tmp})
	require.NoError(t, err)
	require.Equal(t, 2, mockManager.AddMSPCallCount())
}

func TestFolderIdentityLoader_Load_ReadDirError(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.FolderIdentityLoader{IdentityLoader: &idemix.IdentityLoader{KVS: kvss, SignerService: sigService}}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathReturns("./this/path/does/not/exist")
	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)

	err = loader.Load(mockManager, config.MSP{Path: "./this/path/does/not/exist"})
	require.ErrorContains(t, err, "failed reading from")
}

func TestFolderIdentityLoader_Load_MemberLoadError(t *testing.T) { //nolint:paralleltest
	tmp := t.TempDir()
	// An empty directory is not a valid MSP folder, so loading it must fail
	// and the failure must be wrapped with the member's id.
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "broken-member"), 0o755))

	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.FolderIdentityLoader{IdentityLoader: &idemix.IdentityLoader{KVS: kvss, SignerService: sigService}}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathCalls(func(p string) string { return p })
	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)
	mockManager.CacheSizeReturns(10)

	err = loader.Load(mockManager, config.MSP{Path: tmp})
	require.ErrorContains(t, err, "failed to load Idemix MSP configuration [broken-member]")
}
