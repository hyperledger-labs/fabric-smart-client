/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"os"
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

func TestIdentityLoader_Load(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl

	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.IdentityLoader{
		KVS:           kvss,
		SignerService: sigService,
	}

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathReturns("./testdata/idemix")

	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)
	mockManager.CacheSizeReturns(10)

	mspConfig := config.MSP{
		ID:        "idemix",
		MSPType:   idemix.MSPType,
		MSPID:     "idemix",
		Path:      "./testdata/idemix",
		CacheSize: 5,
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

	// Error path: AddMSP fails
	mockManagerAddErr := &mock.Manager{}
	mockManagerAddErr.ConfigReturns(mockConfig)
	mockManagerAddErr.CacheSizeReturns(10)
	mockManagerAddErr.AddMSPReturns(errors.New("add msp failed"))

	err = loader.Load(mockManagerAddErr, mspConfig)
	require.ErrorContains(t, err, "failed adding idemix msp")
}

func TestFolderIdentityLoader_Load(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	loader := &idemix.IdentityLoader{
		KVS:           kvss,
		SignerService: sigService,
	}
	folderLoader := &idemix.FolderIdentityLoader{
		IdentityLoader: loader,
	}

	// Error path: non-existent folder
	mockConfigErr := &mock.Config{}
	mockConfigErr.TranslatePathReturns("./non-existent-path")
	mockManagerErr := &mock.Manager{}
	mockManagerErr.ConfigReturns(mockConfigErr)

	err = folderLoader.Load(mockManagerErr, config.MSP{Path: "./non-existent-path"})
	require.ErrorContains(t, err, "failed reading from")

	// Skip regular files
	tempDirFiles := t.TempDir()
	err = os.WriteFile(filepath.Join(tempDirFiles, "readme.txt"), []byte("ignore me"), 0o600)
	require.NoError(t, err)

	mockConfig := &mock.Config{}
	mockConfig.TranslatePathCalls(func(p string) string { return p })
	mockManager := &mock.Manager{}
	mockManager.ConfigReturns(mockConfig)

	err = folderLoader.Load(mockManager, config.MSP{Path: tempDirFiles})
	require.NoError(t, err)
	require.Equal(t, 0, mockManager.AddMSPCallCount())

	// Success path: loading multi-directory folder
	mockConfigSameIssuer := &mock.Config{}
	mockConfigSameIssuer.TranslatePathCalls(func(p string) string { return p })
	mockManagerSameIssuer := &mock.Manager{}
	mockManagerSameIssuer.ConfigReturns(mockConfigSameIssuer)
	mockManagerSameIssuer.CacheSizeReturns(10)

	err = folderLoader.Load(mockManagerSameIssuer, config.MSP{Path: "./testdata/sameissuer"})
	require.NoError(t, err)
	require.Equal(t, 2, mockManagerSameIssuer.AddMSPCallCount())
	for i, dir := range []string{"idemix", "idemix2"} {
		name, mspType, _, _ := mockManagerSameIssuer.AddMSPArgsForCall(i)
		require.Equal(t, dir, name, "the MSP is named after its directory")
		require.Equal(t, idemix.MSPType, mspType)
	}
	require.Equal(t, 2, mockManagerSameIssuer.AddDeserializerCallCount())

	// Error path: empty subdirectory fails to load MSP configuration
	tempDirEmptySub := t.TempDir()
	err = os.Mkdir(filepath.Join(tempDirEmptySub, "empty-msp"), 0o700)
	require.NoError(t, err)

	mockConfigEmpty := &mock.Config{}
	mockConfigEmpty.TranslatePathCalls(func(p string) string { return p })
	mockManagerEmpty := &mock.Manager{}
	mockManagerEmpty.ConfigReturns(mockConfigEmpty)

	err = folderLoader.Load(mockManagerEmpty, config.MSP{Path: tempDirEmptySub})
	require.ErrorContains(t, err, "failed to load Idemix MSP configuration [empty-msp]")
}
