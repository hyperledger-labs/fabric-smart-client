/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

func TestSetupBCCSPKeystoreConfig(t *testing.T) { //nolint:paralleltest
	keystoreDir := "/tmp"

	// Case 1 : Check with empty FactoryOpts
	rtnConfig := SetupBCCSPKeystoreConfig(nil, keystoreDir)
	require.NotNil(t, rtnConfig)
	require.Equal(t, "SW", rtnConfig.Default)
	require.NotNil(t, rtnConfig.SW)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2 : Check with 'SW' as default provider
	// Case 2-1 : without SwOpts
	bccspConfig := &factory.FactoryOpts{
		Default: "SW",
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.NotNil(t, rtnConfig.SW)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-2 : without SwOpts.FileKeystore
	bccspConfig.SW = &factory.SwOpts{
		Hash:     "SHA2",
		Security: 256,
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-3 : without SwOpts.FileKeystore.KeyStorePath
	bccspConfig.SW = &factory.SwOpts{
		Hash:         "SHA2",
		Security:     256,
		FileKeystore: &factory.FileKeystoreOpts{},
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 2-4 : with empty SwOpts.FileKeystore.KeyStorePath
	bccspConfig.SW = &factory.SwOpts{
		Hash:         "SHA2",
		Security:     256,
		FileKeystore: &factory.FileKeystoreOpts{KeyStorePath: ""},
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)

	// Case 3 : Check with 'PKCS11' as default provider
	// Case 3-1 : without SwOpts
	bccspConfig.Default = "PKCS11"
	bccspConfig.SW = nil
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.Nil(t, rtnConfig.SW)

	// Case 3-2 : without SwOpts.FileKeystore
	bccspConfig.SW = &factory.SwOpts{
		Hash:     "SHA2",
		Security: 256,
	}
	rtnConfig = SetupBCCSPKeystoreConfig(bccspConfig, keystoreDir)
	require.NotNil(t, rtnConfig.SW.FileKeystore)
	require.Equal(t, rtnConfig.SW.FileKeystore.KeyStorePath, keystoreDir)
}

func TestSetupBCCSPKeystoreConfig_DoesNotMutateDefaultOpts(t *testing.T) { //nolint:paralleltest
	defaultOpts := factory.GetDefaultOpts()
	require.NotNil(t, defaultOpts)
	require.NotNil(t, defaultOpts.SW)
	originalSW := *defaultOpts.SW
	var originalKeyStorePath string
	if defaultOpts.SW.FileKeystore != nil {
		originalKeyStorePath = defaultOpts.SW.FileKeystore.KeyStorePath
	}

	rtnConfig := SetupBCCSPKeystoreConfig(nil, "/tmp/fabric-smart-client-test-keystore")

	require.NotNil(t, rtnConfig)
	require.Equal(t, "/tmp/fabric-smart-client-test-keystore", rtnConfig.SW.FileKeystore.KeyStorePath)
	require.Equal(t, originalSW.Hash, defaultOpts.SW.Hash)
	require.Equal(t, originalSW.Security, defaultOpts.SW.Security)
	if defaultOpts.SW.FileKeystore == nil {
		require.Empty(t, originalKeyStorePath)
	} else {
		require.Equal(t, originalKeyStorePath, defaultOpts.SW.FileKeystore.KeyStorePath)
	}
}

func TestSetupBCCSPKeystoreConfig_DoesNotMutateInput(t *testing.T) { //nolint:paralleltest
	bccspConfig := &factory.FactoryOpts{
		Default: "SW",
		SW: &factory.SwOpts{
			Hash:         "SHA2",
			Security:     256,
			FileKeystore: &factory.FileKeystoreOpts{},
		},
	}

	rtnConfig := SetupBCCSPKeystoreConfig(bccspConfig, "/tmp/fabric-smart-client-input-keystore")

	require.NotNil(t, rtnConfig)
	require.Equal(t, "/tmp/fabric-smart-client-input-keystore", rtnConfig.SW.FileKeystore.KeyStorePath)
	require.Empty(t, bccspConfig.SW.FileKeystore.KeyStorePath)
}

func TestGetLocalMspConfig(t *testing.T) { //nolint:paralleltest
	mspDir := "testdata/sampleconfig"
	_, err := GetLocalMspConfig(mspDir, nil, "SampleOrg")
	require.NoError(t, err)
}

func TestGetLocalMspConfigFails(t *testing.T) { //nolint:paralleltest
	_, err := GetLocalMspConfig("/tmp/", nil, "SampleOrg")
	require.Error(t, err)
}

func TestGetPemMaterialFromDirWithFile(t *testing.T) { //nolint:paralleltest
	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "fabric-msp-test")
	require.NoError(t, err)
	err = tempFile.Close()
	require.NoError(t, err)

	_, err = getPemMaterialFromDir(tempFile.Name())
	require.Error(t, err)
}

func TestGetPemMaterialFromDirWithSymlinks(t *testing.T) { //nolint:paralleltest
	mspDir, err := filepath.Abs("testdata/sampleconfig")
	require.NoError(t, err)
	tempDir := t.TempDir()

	dirSymlinkName := filepath.Join(tempDir, "..data")
	err = os.Symlink(filepath.Join(mspDir, "signcerts"), dirSymlinkName)
	require.NoError(t, err)

	fileSymlinkTarget := filepath.Join("..data", "peer.pem")
	fileSymlinkName := filepath.Join(tempDir, "peer.pem")
	err = os.Symlink(fileSymlinkTarget, fileSymlinkName)
	require.NoError(t, err)

	pemdataSymlink, err := getPemMaterialFromDir(tempDir)
	require.NoError(t, err)
	expected, err := getPemMaterialFromDir(filepath.Join(mspDir, "signcerts"))
	require.NoError(t, err)
	require.Equal(t, expected, pemdataSymlink)
}

func TestReadFileUtils(t *testing.T) { //nolint:paralleltest
	// test that reading a file with an empty path doesn't crash
	_, err := readPemFile("")
	require.Error(t, err)

	// test that reading an existing file which is not a PEM file doesn't crash
	_, err = readPemFile("/dev/null")
	require.Error(t, err)
}

func TestGetMspConfigWithType(t *testing.T) { //nolint:paralleltest
	conf, err := GetLocalMspConfigWithType("testdata/sampleconfig", nil, "SampleOrg", ProviderTypeToString(FABRIC))
	require.NoError(t, err)
	require.Equal(t, int32(FABRIC), conf.Type)

	conf, err = GetLocalMspConfigWithType(idemixTestDir, nil, "idemix", ProviderTypeToString(IDEMIX))
	require.NoError(t, err)
	require.Equal(t, int32(IDEMIX), conf.Type)

	conf, err = GetVerifyingMspConfig(idemixTestDir, "idemix", ProviderTypeToString(IDEMIX))
	require.NoError(t, err)
	require.Equal(t, int32(IDEMIX), conf.Type)

	_, err = GetLocalMspConfigWithType("testdata/sampleconfig", nil, "SampleOrg", "unknown")
	require.EqualError(t, err, "unknown MSP type 'unknown'")
	_, err = GetVerifyingMspConfig("testdata/sampleconfig", "SampleOrg", "unknown")
	require.EqualError(t, err, "unknown MSP type 'unknown'")
}

// copySampleConfig returns a writable copy of testdata/sampleconfig.
func copySampleConfig(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "msp")
	require.NoError(t, os.CopyFS(dir, os.DirFS("testdata/sampleconfig")))
	return dir
}

func TestGetMspConfigEmptyCertDirs(t *testing.T) { //nolint:paralleltest
	dir := copySampleConfig(t)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, signcerts)))
	require.NoError(t, os.Mkdir(filepath.Join(dir, signcerts), 0o755))
	_, err := GetLocalMspConfig(dir, nil, "SampleOrg")
	require.ErrorContains(t, err, "could not load a valid signer certificate")
	require.ErrorContains(t, err, "no PEM content found")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, cacerts)))
	require.NoError(t, os.Mkdir(filepath.Join(dir, cacerts), 0o755))
	_, err = GetVerifyingMspConfig(dir, "SampleOrg", ProviderTypeToString(FABRIC))
	require.ErrorContains(t, err, "could not load a valid ca certificate")
	require.ErrorContains(t, err, "no PEM content found")
}

func TestGetMspConfigUnreadableDirs(t *testing.T) { //nolint:paralleltest
	for _, tc := range []struct { //nolint:paralleltest
		dir, msg string
	}{
		{admincerts, "could not load a valid admin certificate"},
		{intermediatecerts, "failed loading intermediate ca certs"},
		{tlscacerts, "failed loading TLS ca certs"},
		{tlsintermediatecerts, "failed loading TLS intermediate ca certs"},
		{crlsfolder, "failed loading crls"},
	} {
		t.Run(tc.dir, func(t *testing.T) { //nolint:paralleltest
			dir := copySampleConfig(t)
			path := filepath.Join(dir, tc.dir)
			require.NoError(t, os.RemoveAll(path))
			require.NoError(t, os.WriteFile(path, nil, 0o600))

			_, err := GetVerifyingMspConfig(dir, "SampleOrg", ProviderTypeToString(FABRIC))
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestGetMspConfigEmptyTLSCACertsSkipsIntermediates(t *testing.T) { //nolint:paralleltest
	dir := copySampleConfig(t)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, tlscacerts)))
	require.NoError(t, os.Mkdir(filepath.Join(dir, tlscacerts), 0o755))

	conf, err := GetVerifyingMspConfig(dir, "SampleOrg", ProviderTypeToString(FABRIC))
	require.NoError(t, err)
	fabricConf := &msp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(conf.Config, fabricConf))
	require.Empty(t, fabricConf.TlsRootCerts)
	require.Empty(t, fabricConf.TlsIntermediateCerts)
}

func TestGetMspConfigBadConfigFile(t *testing.T) { //nolint:paralleltest
	for _, tc := range []struct { //nolint:paralleltest
		name, content, msg string
	}{
		{"invalid yaml", "OrganizationalUnitIdentifiers: [", "failed unmarshalling configuration file"},
		{"missing OU certificate", "OrganizationalUnitIdentifiers:\n  - Certificate: cacerts/missing.pem\n    OrganizationalUnitIdentifier: COP\n", "failed loading OrganizationalUnit certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest
			dir := copySampleConfig(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, configfilename), []byte(tc.content), 0o600))

			_, err := GetVerifyingMspConfig(dir, "SampleOrg", ProviderTypeToString(FABRIC))
			require.ErrorContains(t, err, tc.msg)
		})
	}
}
