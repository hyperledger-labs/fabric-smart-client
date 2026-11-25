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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
)

func TestConfigFileErrors(t *testing.T) {
	t.Parallel()

	// ReadFile on non-existent file
	_, err := idemix.ReadFile("./non-existent-file-xyz")
	require.ErrorContains(t, err, "could not read file")

	// GetLocalMspConfigWithType on non-existent dir
	_, err = idemix.GetLocalMspConfigWithType("./non-existent-dir", "idemix")
	require.ErrorContains(t, err, "cannot get idemix msp config from")

	// GetFabricCAIdemixMspConfig: missing issuer public key
	emptyDir := t.TempDir()
	_, err = idemix.GetFabricCAIdemixMspConfig(emptyDir, "test-ca")
	require.ErrorContains(t, err, "failed to read issuer public key file")

	// GetFabricCAIdemixMspConfig: missing revocation public key
	dirWithIPK := t.TempDir()
	err = os.WriteFile(filepath.Join(dirWithIPK, idemix.ConfigFileIssuerPublicKey), []byte("ipk"), 0o600)
	require.NoError(t, err)
	_, err = idemix.GetFabricCAIdemixMspConfig(dirWithIPK, "test-ca")
	require.ErrorContains(t, err, "failed to read revocation public key file")

	// GetFabricCAIdemixMspConfig: malformed user/SignerConfig JSON
	dirWithBadSigner := t.TempDir()
	err = os.WriteFile(filepath.Join(dirWithBadSigner, idemix.ConfigFileIssuerPublicKey), []byte("ipk"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dirWithBadSigner, idemix.IdemixConfigFileRevocationPublicKey), []byte("revpk"), 0o600)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(dirWithBadSigner, idemix.ConfigDirUser), 0o700)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dirWithBadSigner, idemix.ConfigDirUser, idemix.ConfigFileSigner), []byte("invalid-json"), 0o600)
	require.NoError(t, err)

	_, err = idemix.GetFabricCAIdemixMspConfig(dirWithBadSigner, "test-ca")
	require.ErrorContains(t, err, "failed to json unmarshal signer config read at")

	// GetFabricCAIdemixMspConfig: signer config read error (not IsNotExist)
	dirSignerIsDir := t.TempDir()
	err = os.WriteFile(filepath.Join(dirSignerIsDir, idemix.ConfigFileIssuerPublicKey), []byte("ipk"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dirSignerIsDir, idemix.IdemixConfigFileRevocationPublicKey), []byte("revpk"), 0o600)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(dirSignerIsDir, idemix.ConfigDirUser, idemix.ConfigFileSigner), 0o700)
	require.NoError(t, err)

	_, err = idemix.GetFabricCAIdemixMspConfig(dirSignerIsDir, "test-ca")
	require.ErrorContains(t, err, "failed to read the content of signer config at")

	// GetFabricCAIdemixMspConfig: success without signer config
	dirNoSigner := t.TempDir()
	err = os.WriteFile(filepath.Join(dirNoSigner, idemix.ConfigFileIssuerPublicKey), []byte("ipk"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dirNoSigner, idemix.IdemixConfigFileRevocationPublicKey), []byte("revpk"), 0o600)
	require.NoError(t, err)

	conf, err := idemix.GetFabricCAIdemixMspConfig(dirNoSigner, "test-ca")
	require.NoError(t, err)
	require.NotNil(t, conf)
}
