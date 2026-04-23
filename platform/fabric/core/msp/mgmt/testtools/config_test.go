/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msptesttools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/mgmt"
)

func TestFakeSetup(t *testing.T) {
	t.Parallel()
	err := LoadMSPSetupForTesting()
	if err != nil {
		t.Fatalf("LoadLocalMsp failed, err %s", err)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	_, err = mgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed, err %s", err)
	}

	msps, err := mgmt.GetManagerForChain("testchannelid").GetMSPs()
	if err != nil {
		t.Fatalf("EnlistedMSPs failed, err %s", err)
	}

	if len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager for chain %s", "testchannelid")
	}
}

func TestLoadMSPSetupForTestingFromDir_InvalidPath(t *testing.T) {
	t.Parallel()
	err := LoadMSPSetupForTestingFromDir("/nonexistent/path/to/msp")
	require.Error(t, err, "expected error for non-existent MSP directory")
}

func TestLoadMSPSetupForTestingFromDir_InvalidCerts(t *testing.T) {
	t.Parallel()
	// Create a fake MSP directory with valid PEM structure but invalid cert content.
	// This should pass GetLocalMspConfig (which only reads PEM bytes) but fail on Setup
	// (which actually parses the certificates).
	dir := t.TempDir()

	// Create required subdirectories
	for _, sub := range []string{"cacerts", "signcerts", "keystore"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o755))
	}

	// Write a valid PEM block with garbage certificate data
	fakePEM := []byte("-----BEGIN CERTIFICATE-----\nZmFrZWNlcnQ=\n-----END CERTIFICATE-----\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cacerts", "ca.pem"), fakePEM, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "signcerts", "sign.pem"), fakePEM, 0o644))

	err := LoadMSPSetupForTestingFromDir(dir)
	require.Error(t, err, "expected error when setting up MSP with invalid certificates")
}
