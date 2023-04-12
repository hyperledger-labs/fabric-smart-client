/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"

	csp2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/csp"
)

const (
	CLIENT = iota
	ORDERER
	PEER
	ADMIN
)

var (
	// AttrOID is the ASN.1 object identifier for an attribute extension in an
	// X509 certificate
	AttrOID = asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 7, 8, 1}
	// AttrOIDString is the string version of AttrOID
	AttrOIDString = "1.2.3.4.5.6.7.8.1"
)

type Attributes struct {
	Attrs map[string]string `json:"attrs"`
}

type CA struct {
	Name               string
	Country            string
	Province           string
	Locality           string
	OrganizationalUnit string
	StreetAddress      string
	PostalCode         string
	Signer             crypto.Signer
	SignCert           *x509.Certificate
}

// NewCA creates an instance of CA and saves the signing key pair in
// baseDir/name
func NewCA(
	baseDir,
	org,
	name,
	country,
	province,
	locality,
	orgUnit,
	streetAddress,
	postalCode string,
) (*CA, error) {

	var ca *CA

	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		return nil, err
	}

	priv, err := csp2.GeneratePrivateKey(baseDir)
	if err != nil {
		return nil, err
	}

	template := x509Template()
	//this is a CA
	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageDigitalSignature |
		x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign |
		x509.KeyUsageCRLSign
	template.ExtKeyUsage = []x509.ExtKeyUsage{
		x509.ExtKeyUsageClientAuth,
		x509.ExtKeyUsageServerAuth,
	}

	//set the organization for the subject
	subject := subjectTemplateAdditional(country, province, locality, orgUnit, streetAddress, postalCode)
	subject.Organization = []string{org}
	subject.CommonName = name

	template.Subject = subject
	template.SubjectKeyId = computeSKI(priv)

	x509Cert, err := genCertificateECDSA(
		baseDir,
		name,
		&template,
		&template,
		&priv.PublicKey,
		priv,
	)
	if err != nil {
		return nil, err
	}
	ca = &CA{
		Name: name,
		Signer: &csp2.ECDSASigner{
			PrivateKey: priv,
		},
		SignCert:           x509Cert,
		Country:            country,
		Province:           province,
		Locality:           locality,
		OrganizationalUnit: orgUnit,
		StreetAddress:      streetAddress,
		PostalCode:         postalCode,
	}

	return ca, err
}

func LoadCA(baseDir string) (*CA, error) {
	var ca *CA

	// check baseDir exist
	_, err := os.Stat(baseDir)
	if err != nil {
		return nil, err
	}

	// Load private key
	priv, err := csp2.LoadPrivateKey(baseDir)
	if err != nil {
		return nil, err
	}

	// load certificate
	x509Cert, err := LoadCertificateECDSA(baseDir)
	if err != nil {
		return nil, err
	}

	// setup ca
	ca = &CA{
		Name: x509Cert.Subject.CommonName,
		Signer: &csp2.ECDSASigner{
			PrivateKey: priv,
		},
		SignCert: x509Cert,
	}
	if len(x509Cert.Subject.Country) > 0 {
		ca.Country = x509Cert.Subject.Country[0]
	}
	if len(x509Cert.Subject.Province) > 0 {
		ca.Province = x509Cert.Subject.Province[0]
	}
	if len(x509Cert.Subject.Locality) > 0 {
		ca.Locality = x509Cert.Subject.Locality[0]
	}
	if len(x509Cert.Subject.OrganizationalUnit) > 0 {
		ca.OrganizationalUnit = x509Cert.Subject.OrganizationalUnit[0]
	}
	if len(x509Cert.Subject.StreetAddress) > 0 {
		ca.StreetAddress = x509Cert.Subject.StreetAddress[0]
	}
	if len(x509Cert.Subject.PostalCode) > 0 {
		ca.PostalCode = x509Cert.Subject.PostalCode[0]
	}

	return ca, err
}

// SignCertificate creates a signed certificate based on a built-in template
// and saves it in baseDir/name
func (ca *CA) SignCertificate(baseDir, name string, orgUnits, alternateNames []string, pub *ecdsa.PublicKey, ku x509.KeyUsage, eku []x509.ExtKeyUsage, nodeType int) (*x509.Certificate, error) {

	template := x509Template()
	template.KeyUsage = ku
	template.ExtKeyUsage = eku

	//set the organization for the subject
	subject := subjectTemplateAdditional(
		ca.Country,
		ca.Province,
		ca.Locality,
		ca.OrganizationalUnit,
		ca.StreetAddress,
		ca.PostalCode,
	)
	subject.CommonName = name

	subject.OrganizationalUnit = append(subject.OrganizationalUnit, orgUnits...)

	template.Subject = subject
	for _, san := range alternateNames {
		// try to parse as an IP address first
		ip := net.ParseIP(san)
		if ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, san)
		}
	}

	// Handle attributes
	hfType := ""
	if nodeType == CLIENT {
		hfType = "client"
	}
	// TODO: remove this by introducing custom attributes
	relay := "false"
	if strings.Contains(strings.ToLower(name), "relay") {
		relay = "true"
	}
	attrs := &Attributes{
		map[string]string{
			"hf.Affiliation":  ca.OrganizationalUnit,
			"hf.EnrollmentID": name,
			"hf.Type":         hfType,
			"relay":           relay,
		},
	}
	buf, err := json.Marshal(attrs)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal attributes")
	}
	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       AttrOID,
		Critical: false,
		Value:    buf,
	})

	cert, err := genCertificateECDSA(
		baseDir,
		name,
		&template,
		ca.SignCert,
		pub,
		ca.Signer,
	)

	if err != nil {
		return nil, err
	}

	return cert, nil
}

// compute Subject Key Identifier
func computeSKI(privKey *ecdsa.PrivateKey) []byte {
	// Marshall the public key
	raw := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)

	// Hash it
	hash := sha256.Sum256(raw)
	return hash[:]
}

// default template for X509 subject
func subjectTemplate() pkix.Name {
	return pkix.Name{
		Country:  []string{"US"},
		Locality: []string{"San Francisco"},
		Province: []string{"California"},
	}
}

// Additional for X509 subject
func subjectTemplateAdditional(
	country,
	province,
	locality,
	orgUnit,
	streetAddress,
	postalCode string,
) pkix.Name {
	name := subjectTemplate()
	if len(country) >= 1 {
		name.Country = []string{country}
	}
	if len(province) >= 1 {
		name.Province = []string{province}
	}

	if len(locality) >= 1 {
		name.Locality = []string{locality}
	}
	if len(orgUnit) >= 1 {
		name.OrganizationalUnit = []string{orgUnit}
	}
	if len(streetAddress) >= 1 {
		name.StreetAddress = []string{streetAddress}
	}
	if len(postalCode) >= 1 {
		name.PostalCode = []string{postalCode}
	}
	return name
}

// default template for X509 certificates
func x509Template() x509.Certificate {

	// generate a serial number
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)

	// set expiry to around 10 years
	expiry := 3650 * 24 * time.Hour
	// round minute and backdate 5 minutes
	notBefore := time.Now().Round(time.Minute).Add(-5 * time.Minute).UTC()

	//basic template to use
	x509 := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notBefore.Add(expiry).UTC(),
		BasicConstraintsValid: true,
	}
	return x509

}

// generate a signed X509 certificate using ECDSA
func genCertificateECDSA(
	baseDir,
	name string,
	template,
	parent *x509.Certificate,
	pub *ecdsa.PublicKey,
	priv interface{},
) (*x509.Certificate, error) {

	//create the x509 public cert
	certBytes, err := x509.CreateCertificate(rand.Reader, template, parent, pub, priv)
	if err != nil {
		return nil, err
	}

	//write cert out to file
	fileName := filepath.Join(baseDir, name+"-cert.pem")
	certFile, err := os.Create(fileName)
	if err != nil {
		return nil, err
	}
	//pem encode the cert
	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	certFile.Close()
	if err != nil {
		return nil, err
	}

	x509Cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}
	return x509Cert, nil
}

// LoadCertificateECDSA load a ecdsa cert from a file in cert path
func LoadCertificateECDSA(certPath string) (*x509.Certificate, error) {
	var cert *x509.Certificate
	var err error

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".pem") {
			rawCert, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			block, _ := pem.Decode(rawCert)
			if block == nil || block.Type != "CERTIFICATE" {
				return errors.Errorf("%s: wrong PEM encoding", path)
			}
			cert, err = x509.ParseCertificate(block.Bytes)
			if err != nil {
				return errors.Errorf("%s: wrong DER encoding", path)
			}
		}
		return nil
	}

	err = filepath.Walk(certPath, walkFunc)
	if err != nil {
		return nil, err
	}

	return cert, err
}
