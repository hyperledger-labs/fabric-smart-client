/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/cryptogen/ca"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/cryptogen/msp"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

const (
	testCAOrg              = "example.com"
	testCAName             = "ca" + "." + testCAOrg
	testName               = "peer0"
	testCountry            = "US"
	testProvince           = "California"
	testLocality           = "San Francisco"
	testOrganizationalUnit = "Hyperledger Fabric"
	testStreetAddress      = "testStreetAddress"
	testPostalCode         = "123456"
)

func testGenerateLocalMSP(t *testing.T, nodeOUs bool) {
	t.Helper()
	testDir := t.TempDir()

	err := msp.GenerateLocalMSP(testDir, testName, nil, &ca.CA{}, &ca.CA{}, msp.PEER, nodeOUs, false, nil)
	require.Error(t, err, "Empty CA should have failed")

	caDir := filepath.Join(testDir, "ca")
	tlsCADir := filepath.Join(testDir, "tlsca")
	mspDir := filepath.Join(testDir, "msp")
	tlsDir := filepath.Join(testDir, "tls")

	// generate signing CA
	signCA, err := ca.NewCA(caDir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	require.NoError(t, err, "Error generating CA")
	// generate TLS CA
	tlsCA, err := ca.NewCA(tlsCADir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	require.NoError(t, err, "Error generating CA")

	assert.NotEmpty(t, signCA.SignCert.Subject.Country, "country cannot be empty.")
	assert.Equal(t, testCountry, signCA.SignCert.Subject.Country[0], "Failed to match country")
	assert.NotEmpty(t, signCA.SignCert.Subject.Province, "province cannot be empty.")
	assert.Equal(t, testProvince, signCA.SignCert.Subject.Province[0], "Failed to match province")
	assert.NotEmpty(t, signCA.SignCert.Subject.Locality, "locality cannot be empty.")
	assert.Equal(t, testLocality, signCA.SignCert.Subject.Locality[0], "Failed to match locality")
	assert.NotEmpty(t, signCA.SignCert.Subject.OrganizationalUnit, "organizationalUnit cannot be empty.")
	assert.Equal(t, testOrganizationalUnit, signCA.SignCert.Subject.OrganizationalUnit[0], "Failed to match organizationalUnit")
	assert.NotEmpty(t, signCA.SignCert.Subject.StreetAddress, "streetAddress cannot be empty.")
	assert.Equal(t, testStreetAddress, signCA.SignCert.Subject.StreetAddress[0], "Failed to match streetAddress")
	assert.NotEmpty(t, signCA.SignCert.Subject.PostalCode, "postalCode cannot be empty.")
	assert.Equal(t, testPostalCode, signCA.SignCert.Subject.PostalCode[0], "Failed to match postalCode")

	// generate local MSP for nodeType=PEER
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp.PEER, nodeOUs, false, nil)
	require.NoError(t, err, "Failed to generate local MSP")

	// check to see that the right files were generated/saved
	mspFiles := []string{
		filepath.Join(mspDir, "cacerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "tlscacerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "keystore"),
		filepath.Join(mspDir, "signcerts", testName+"-cert.pem"),
	}
	if nodeOUs {
		mspFiles = append(mspFiles, filepath.Join(mspDir, "config.yaml"))
	} else {
		mspFiles = append(mspFiles, filepath.Join(mspDir, "admincerts", testName+"-cert.pem"))
	}

	tlsFiles := []string{
		filepath.Join(tlsDir, "ca.crt"),
		filepath.Join(tlsDir, "server.key"),
		filepath.Join(tlsDir, "server.crt"),
	}

	for _, file := range mspFiles {
		assert.True(t, checkForFile(file),
			"Expected to find file "+file)
	}
	for _, file := range tlsFiles {
		assert.True(t, checkForFile(file),
			"Expected to find file "+file)
	}

	// generate local MSP for nodeType=CLIENT
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp.CLIENT, nodeOUs, false, nil)
	require.NoError(t, err, "Failed to generate local MSP")
	// check all
	for _, file := range mspFiles {
		assert.True(t, checkForFile(file),
			"Expected to find file "+file)
	}

	for _, file := range tlsFiles {
		assert.True(t, checkForFile(file),
			"Expected to find file "+file)
	}

	tlsCA.Name = "test/fail"
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp.CLIENT, nodeOUs, false, nil)
	require.Error(t, err, "Should have failed with CA name 'test/fail'")
	signCA.Name = "test/fail"
	err = msp.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp.ORDERER, nodeOUs, false, nil)
	require.Error(t, err, "Should have failed with CA name 'test/fail'")
	t.Log(err)
}

func TestGenerateLocalMSPWithNodeOU(t *testing.T) { //nolint:paralleltest
	testGenerateLocalMSP(t, true)
}

func TestGenerateLocalMSPWithoutNodeOU(t *testing.T) { //nolint:paralleltest
	testGenerateLocalMSP(t, false)
}

func testGenerateVerifyingMSP(t *testing.T, nodeOUs bool) {
	t.Helper()
	testDir := t.TempDir()
	caDir := filepath.Join(testDir, "ca")
	tlsCADir := filepath.Join(testDir, "tlsca")
	mspDir := filepath.Join(testDir, "msp")
	// generate signing CA
	signCA, err := ca.NewCA(caDir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	require.NoError(t, err, "Error generating CA")
	// generate TLS CA
	tlsCA, err := ca.NewCA(tlsCADir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	require.NoError(t, err, "Error generating CA")

	err = msp.GenerateVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)
	require.NoError(t, err, "Failed to generate verifying MSP")

	// check to see that the right files were generated/saved
	files := []string{
		filepath.Join(mspDir, "cacerts", testCAName+"-cert.pem"),
		filepath.Join(mspDir, "tlscacerts", testCAName+"-cert.pem"),
	}

	if nodeOUs {
		files = append(files, filepath.Join(mspDir, "config.yaml"))
	} else {
		files = append(files, filepath.Join(mspDir, "admincerts", testCAName+"-cert.pem"))
	}

	for _, file := range files {
		assert.True(t, checkForFile(file),
			"Expected to find file "+file)
	}

	tlsCA.Name = "test/fail"
	err = msp.GenerateVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)
	require.Error(t, err, "Should have failed with CA name 'test/fail'")
	signCA.Name = "test/fail"
	err = msp.GenerateVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)
	require.Error(t, err, "Should have failed with CA name 'test/fail'")
	t.Log(err)
}

func TestGenerateVerifyingMSPWithNodeOU(t *testing.T) { //nolint:paralleltest
	testGenerateVerifyingMSP(t, true)
}

func TestGenerateVerifyingMSPWithoutNodeOU(t *testing.T) { //nolint:paralleltest
	testGenerateVerifyingMSP(t, true)
}

func TestExportConfig(t *testing.T) { //nolint:paralleltest
	testDir := t.TempDir()
	path := filepath.Join(testDir, "export-test")
	configFile := filepath.Join(path, "config.yaml")
	caFile := "ca.pem"
	t.Log(path)
	err := os.MkdirAll(path, 0o755)
	require.NoError(t, err, "failed to create test directory")

	err = msp.ExportConfig(path, caFile, true)
	require.NoError(t, err)

	configBytes, err := os.ReadFile(configFile)
	require.NoError(t, err, "failed to read config file")

	config := &fabricmsp.Configuration{}
	err = yaml.Unmarshal(configBytes, config)
	require.NoError(t, err, "failed to unmarshal config")
	assert.True(t, config.NodeOUs.Enable)
	assert.Equal(t, caFile, config.NodeOUs.ClientOUIdentifier.Certificate)
	assert.Equal(t, msp.CLIENTOU, config.NodeOUs.ClientOUIdentifier.OrganizationalUnitIdentifier)
	assert.Equal(t, caFile, config.NodeOUs.PeerOUIdentifier.Certificate)
	assert.Equal(t, msp.PEEROU, config.NodeOUs.PeerOUIdentifier.OrganizationalUnitIdentifier)
	assert.Equal(t, caFile, config.NodeOUs.AdminOUIdentifier.Certificate)
	assert.Equal(t, msp.ADMINOU, config.NodeOUs.AdminOUIdentifier.OrganizationalUnitIdentifier)
	assert.Equal(t, caFile, config.NodeOUs.OrdererOUIdentifier.Certificate)
	assert.Equal(t, msp.ORDEREROU, config.NodeOUs.OrdererOUIdentifier.OrganizationalUnitIdentifier)
}

func checkForFile(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
