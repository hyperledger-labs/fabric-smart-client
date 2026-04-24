/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	mspdrivermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
)

func TestIdentityLoader_Load(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mSignerSvc := &mspdrivermock.SignerService{}
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.SignerServiceReturns(mSignerSvc)

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "./testdata/msp",
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddDeserializerCallCount())
	require.Equal(t, 1, mMgr.AddMSPCallCount())
	name, _, _, _ := mMgr.AddMSPArgsForCall(0)
	require.Equal(t, "test-id", name)
	require.Equal(t, 1, mMgr.SetDefaultIdentityCallCount())
}

func TestIdentityLoader_Load_WithMSPSubdir(t *testing.T) {
	t.Parallel()
	// Create a directory where the MSP config is at path/msp
	dir := t.TempDir()
	mspDir := filepath.Join(dir, "myid", "msp")

	// Copy testdata into mspDir
	for _, sub := range []string{"cacerts", "signcerts", "keystore", "admincerts"} {
		require.NoError(t, os.MkdirAll(filepath.Join(mspDir, sub), 0o755))
	}
	copyFile(t, filepath.Join("testdata", "msp", "cacerts", "ca.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "cacerts", "ca.pem"))
	copyFile(t, filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "signcerts", "cert.pem"))
	copyFile(t, filepath.Join("testdata", "msp", "keystore", "priv_sk"),
		filepath.Join(mspDir, "keystore", "priv_sk"))
	copyFile(t, filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "admincerts", "admin.pem"))

	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.SignerServiceReturns(&mspdrivermock.SignerService{})

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "myid",
		MSPID: "apple",
		Path:  filepath.Join(dir, "myid"),
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddMSPCallCount())
}

func TestIdentityLoader_Load_InvalidPath(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "msp1",
		Path:  "/nonexistent/path",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load BCCSP MSP configuration")
}

func TestIdentityLoader_Load_WithOpts(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "./testdata/msp",
		Opts: map[interface{}]interface{}{
			"bccsp": map[string]interface{}{"Default": "SW"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddMSPCallCount())
}

func TestIdentityLoader_Load_WithBadOpts(t *testing.T) {
	t.Parallel()
	mMgr := &mspdrivermock.Manager{}
	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "ignored",
		Opts: map[interface{}]interface{}{
			"bccsp": "invalid-not-a-map",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal BCCSP opts")
}

func TestIdentityLoader_Load_AddMSPError(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.AddMSPReturns(errors.New("mock AddMSP error"))

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "./testdata/msp",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed adding MSP")
}

func TestFolderIdentityLoader_Load(t *testing.T) {
	t.Parallel()
	// Create folder structure with one MSP subdirectory
	dir := t.TempDir()
	mspDir := filepath.Join(dir, "org1", "msp")

	for _, sub := range []string{"cacerts", "signcerts", "keystore", "admincerts"} {
		require.NoError(t, os.MkdirAll(filepath.Join(mspDir, sub), 0o755))
	}
	copyFile(t, filepath.Join("testdata", "msp", "cacerts", "ca.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "cacerts", "ca.pem"))
	copyFile(t, filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "signcerts", "cert.pem"))
	copyFile(t, filepath.Join("testdata", "msp", "keystore", "priv_sk"),
		filepath.Join(mspDir, "keystore", "priv_sk"))
	copyFile(t, filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "admincerts", "admin.pem"))

	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.SignerServiceReturns(&mspdrivermock.SignerService{})

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: dir,
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddMSPCallCount())
	name, _, _, _ := mMgr.AddMSPArgsForCall(0)
	require.Equal(t, "org1", name)
}

func TestFolderIdentityLoader_Load_InvalidDir(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: "/nonexistent/dir",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed reading from")
}

func TestFolderIdentityLoader_Load_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: dir,
	})
	require.NoError(t, err)
	require.Equal(t, 0, mMgr.AddMSPCallCount())
}

func TestFolderIdentityLoader_Load_SkipsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notadir.txt"), []byte("data"), 0o644))

	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: dir,
	})
	require.NoError(t, err)
	require.Equal(t, 0, mMgr.AddMSPCallCount())
}
