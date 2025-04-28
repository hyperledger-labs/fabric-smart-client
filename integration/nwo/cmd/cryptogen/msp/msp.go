/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"

	ca2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/ca"
	csp2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/csp"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/pkcs11"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	fabricmsp "github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	CLIENT = iota
	ORDERER
	PEER
	ADMIN
)

const (
	CLIENTOU  = "client"
	PEEROU    = "peer"
	ADMINOU   = "admin"
	ORDEREROU = "orderer"
)

var nodeOUMap = map[int]string{
	CLIENT:  CLIENTOU,
	PEER:    PEEROU,
	ADMIN:   ADMINOU,
	ORDERER: ORDEREROU,
}

func GenerateLocalMSP(baseDir, name string, sans []string, signCA, tlsCA *ca2.CA, nodeType int, nodeOUs, hsm bool, pk *ecdsa.PublicKey) error {
	// create folder structure
	mspDir := filepath.Join(baseDir, "msp")
	tlsDir := filepath.Join(baseDir, "tls")

	err := createFolderStructure(mspDir, true)
	if err != nil {
		return err
	}

	err = os.MkdirAll(tlsDir, 0755)
	if err != nil {
		return err
	}

	/*
		Create the MSP identity artifacts
	*/

	// generate private key
	if pk == nil {
		if hsm {
			pk, err = pkcs11.GeneratePrivateKey()
			if err != nil {
				return err
			}
		} else {
			// get keystore path
			keystore := filepath.Join(mspDir, "keystore")
			priv, err := csp2.GeneratePrivateKey(keystore)
			if err != nil {
				return err
			}
			pk = &priv.PublicKey
		}
	}

	// generate X509 certificate using signing CA
	var ous []string
	if nodeOUs {
		ous = []string{nodeOUMap[nodeType]}
	}
	cert, err := signCA.SignCertificate(
		filepath.Join(mspDir, "signcerts"),
		name,
		ous,
		nil,
		pk,
		x509.KeyUsageDigitalSignature,
		[]x509.ExtKeyUsage{},
		nodeType,
	)
	if err != nil {
		return err
	}

	// write artifacts to MSP folders

	// the signing CA certificate goes into cacerts
	err = x509Export(
		filepath.Join(mspDir, "cacerts", x509Filename(signCA.Name)),
		signCA.SignCert,
	)
	if err != nil {
		return err
	}
	// the TLS CA certificate goes into tlscacerts
	err = x509Export(
		filepath.Join(mspDir, "tlscacerts", x509Filename(tlsCA.Name)),
		tlsCA.SignCert,
	)
	if err != nil {
		return err
	}

	// generate config.yaml if required
	if nodeOUs {
		if err := exportConfig(mspDir, filepath.Join("cacerts", x509Filename(signCA.Name)), true); err != nil {
			return errors.WithMessagef(err, "failed to export cacerts config")
		}
	}

	// the signing identity goes into admincerts.
	// This means that the signing identity
	// of this MSP is also an admin of this MSP
	// NOTE: the admincerts folder is going to be
	// cleared up anyway by copyAdminCert, but
	// we leave a valid admin for now for the sake
	// of unit tests
	if !nodeOUs {
		err = x509Export(filepath.Join(mspDir, "admincerts", x509Filename(name)), cert)
		if err != nil {
			return err
		}
	}

	/*
		Generate the TLS artifacts in the TLS folder
	*/

	// generate private key
	tlsPrivKey, err := csp2.GeneratePrivateKey(tlsDir)
	if err != nil {
		return err
	}

	// generate X509 certificate using TLS CA
	_, err = tlsCA.SignCertificate(filepath.Join(tlsDir), name, nil, sans, &tlsPrivKey.PublicKey, x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth}, 0)
	if err != nil {
		return err
	}
	err = x509Export(filepath.Join(tlsDir, "ca.crt"), tlsCA.SignCert)
	if err != nil {
		return err
	}

	// rename the generated TLS X509 cert
	tlsFilePrefix := "server"
	if nodeType == CLIENT || nodeType == ADMIN {
		tlsFilePrefix = "client"
	}
	err = os.Rename(filepath.Join(tlsDir, x509Filename(name)),
		filepath.Join(tlsDir, tlsFilePrefix+".crt"))
	if err != nil {
		return err
	}

	err = keyExport(tlsDir, filepath.Join(tlsDir, tlsFilePrefix+".key"))
	if err != nil {
		return err
	}

	return nil
}

func GenerateVerifyingMSP(
	baseDir string,
	signCA,
	tlsCA *ca2.CA,
	nodeOUs bool,
) error {

	// create folder structure and write artifacts to proper locations
	err := createFolderStructure(baseDir, false)
	if err != nil {
		return err
	}
	// the signing CA certificate goes into cacerts
	err = x509Export(
		filepath.Join(baseDir, "cacerts", x509Filename(signCA.Name)),
		signCA.SignCert,
	)
	if err != nil {
		return err
	}
	// the TLS CA certificate goes into tlscacerts
	err = x509Export(
		filepath.Join(baseDir, "tlscacerts", x509Filename(tlsCA.Name)),
		tlsCA.SignCert,
	)
	if err != nil {
		return err
	}

	// generate config.yaml if required
	if nodeOUs {
		if err := exportConfig(baseDir, "cacerts/"+x509Filename(signCA.Name), true); err != nil {
			return errors.WithMessagef(err, "failed to export cacerts config")
		}
	}

	// create a throwaway cert to act as an admin cert
	// NOTE: the admincerts folder is going to be
	// cleared up anyway by copyAdminCert, but
	// we leave a valid admin for now for the sake
	// of unit tests
	if nodeOUs {
		return nil
	}

	ksDir := filepath.Join(baseDir, "keystore")
	err = os.Mkdir(ksDir, 0755)
	defer utils.IgnoreErrorWithOneArg(os.RemoveAll, ksDir)
	if err != nil {
		return errors.WithMessage(err, "failed to create keystore directory")
	}
	priv, err := csp2.GeneratePrivateKey(ksDir)
	if err != nil {
		return err
	}
	_, err = signCA.SignCertificate(filepath.Join(baseDir, "admincerts"), signCA.Name, nil, nil, &priv.PublicKey, x509.KeyUsageDigitalSignature, []x509.ExtKeyUsage{}, 0)
	if err != nil {
		return err
	}

	return nil
}

func createFolderStructure(rootDir string, local bool) error {

	var folders []string
	// create admincerts, cacerts, keystore and signcerts folders
	folders = []string{
		filepath.Join(rootDir, "admincerts"),
		filepath.Join(rootDir, "cacerts"),
		filepath.Join(rootDir, "tlscacerts"),
	}
	if local {
		folders = append(folders, filepath.Join(rootDir, "keystore"),
			filepath.Join(rootDir, "signcerts"))
	}

	for _, folder := range folders {
		err := os.MkdirAll(folder, 0755)
		if err != nil {
			return err
		}
	}

	return nil
}

func x509Filename(name string) string {
	return name + "-cert.pem"
}

func x509Export(path string, cert *x509.Certificate) error {
	return pemExport(path, "CERTIFICATE", cert.Raw)
}

func keyExport(keystore, output string) error {
	return os.Rename(filepath.Join(keystore, "priv_sk"), output)
}

func pemExport(path, pemType string, bytes []byte) error {
	// write pem out to file
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer utils.IgnoreErrorFunc(file.Close)

	return pem.Encode(file, &pem.Block{Type: pemType, Bytes: bytes})
}

func exportConfig(mspDir, caFile string, enable bool) error {
	var config = &fabricmsp.Configuration{
		NodeOUs: &fabricmsp.NodeOUs{
			Enable: enable,
			ClientOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: CLIENTOU,
			},
			PeerOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: PEEROU,
			},
			AdminOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: ADMINOU,
			},
			OrdererOUIdentifier: &fabricmsp.OrganizationalUnitIdentifiersConfiguration{
				Certificate:                  caFile,
				OrganizationalUnitIdentifier: ORDEREROU,
			},
		},
	}

	configBytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(mspDir, "config.yaml"))
	if err != nil {
		return err
	}

	defer utils.IgnoreErrorFunc(file.Close)
	_, err = file.WriteString(string(configBytes))

	return err
}
