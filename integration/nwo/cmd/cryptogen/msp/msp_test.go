/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp_test

import (
	"os"
	"path/filepath"
	"testing"

	ca2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/ca"
	msp2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/msp"
	fabricmsp "github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
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

var testDir = filepath.Join(os.TempDir(), "msp-test")

func testGenerateLocalMSP(t *testing.T, nodeOUs bool) {
	cleanup(testDir)

	err := msp2.GenerateLocalMSP(testDir, testName, nil, &ca2.CA{}, &ca2.CA{}, msp2.PEER, nodeOUs, false, nil)
	assert.Error(t, err, "Empty CA should have failed")

	caDir := filepath.Join(testDir, "ca")
	tlsCADir := filepath.Join(testDir, "tlsca")
	mspDir := filepath.Join(testDir, "msp")
	tlsDir := filepath.Join(testDir, "tls")

	// generate signing CA
	signCA, err := ca2.NewCA(caDir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")
	// generate TLS CA
	tlsCA, err := ca2.NewCA(tlsCADir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")

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
	err = msp2.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp2.PEER, nodeOUs, false, nil)
	assert.NoError(t, err, "Failed to generate local MSP")

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
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}
	for _, file := range tlsFiles {
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}

	// generate local MSP for nodeType=CLIENT
	err = msp2.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp2.CLIENT, nodeOUs, false, nil)
	assert.NoError(t, err, "Failed to generate local MSP")
	// check all
	for _, file := range mspFiles {
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}

	for _, file := range tlsFiles {
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}

	tlsCA.Name = "test/fail"
	err = msp2.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp2.CLIENT, nodeOUs, false, nil)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	signCA.Name = "test/fail"
	err = msp2.GenerateLocalMSP(testDir, testName, nil, signCA, tlsCA, msp2.ORDERER, nodeOUs, false, nil)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	t.Log(err)
	cleanup(testDir)
}

func TestGenerateLocalMSPWithNodeOU(t *testing.T) {
	testGenerateLocalMSP(t, true)
}

func TestGenerateLocalMSPWithoutNodeOU(t *testing.T) {
	testGenerateLocalMSP(t, false)
}

func testGenerateVerifyingMSP(t *testing.T, nodeOUs bool) {
	caDir := filepath.Join(testDir, "ca")
	tlsCADir := filepath.Join(testDir, "tlsca")
	mspDir := filepath.Join(testDir, "msp")
	// generate signing CA
	signCA, err := ca2.NewCA(caDir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")
	// generate TLS CA
	tlsCA, err := ca2.NewCA(tlsCADir, testCAOrg, testCAName, testCountry, testProvince, testLocality, testOrganizationalUnit, testStreetAddress, testPostalCode)
	assert.NoError(t, err, "Error generating CA")

	err = msp2.GenerateVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)
	assert.NoError(t, err, "Failed to generate verifying MSP")

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
		assert.Equal(t, true, checkForFile(file),
			"Expected to find file "+file)
	}

	tlsCA.Name = "test/fail"
	err = msp2.GenerateVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	signCA.Name = "test/fail"
	err = msp2.GenerateVerifyingMSP(mspDir, signCA, tlsCA, nodeOUs)
	assert.Error(t, err, "Should have failed with CA name 'test/fail'")
	t.Log(err)
	cleanup(testDir)

}

func TestGenerateVerifyingMSPWithNodeOU(t *testing.T) {
	testGenerateVerifyingMSP(t, true)
}

func TestGenerateVerifyingMSPWithoutNodeOU(t *testing.T) {
	testGenerateVerifyingMSP(t, true)
}

func TestExportConfig(t *testing.T) {
	path := filepath.Join(testDir, "export-test")
	configFile := filepath.Join(path, "config.yaml")
	caFile := "ca.pem"
	t.Log(path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		t.Fatalf("failed to create test directory: [%s]", err)
	}

	err = msp2.ExportConfig(path, caFile, true)
	assert.NoError(t, err)

	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read config file: [%s]", err)
	}

	config := &fabricmsp.Configuration{}
	err = yaml.Unmarshal(configBytes, config)
	if err != nil {
		t.Fatalf("failed to unmarshal config: [%s]", err)
	}
	assert.True(t, config.NodeOUs.Enable)
	assert.Equal(t, caFile, config.NodeOUs.ClientOUIdentifier.Certificate)
	assert.Equal(t, msp2.CLIENTOU, config.NodeOUs.ClientOUIdentifier.OrganizationalUnitIdentifier)
	assert.Equal(t, caFile, config.NodeOUs.PeerOUIdentifier.Certificate)
	assert.Equal(t, msp2.PEEROU, config.NodeOUs.PeerOUIdentifier.OrganizationalUnitIdentifier)
	assert.Equal(t, caFile, config.NodeOUs.AdminOUIdentifier.Certificate)
	assert.Equal(t, msp2.ADMINOU, config.NodeOUs.AdminOUIdentifier.OrganizationalUnitIdentifier)
	assert.Equal(t, caFile, config.NodeOUs.OrdererOUIdentifier.Certificate)
	assert.Equal(t, msp2.ORDEREROU, config.NodeOUs.OrdererOUIdentifier.OrganizationalUnitIdentifier)
}

func cleanup(dir string) {
	os.RemoveAll(dir)
}

func checkForFile(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
