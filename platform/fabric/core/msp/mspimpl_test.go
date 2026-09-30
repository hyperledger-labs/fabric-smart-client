/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
)

// setupFabricMSP sets up an MSP of the given version from conf.
func setupFabricMSP(t *testing.T, version MSPVersion, conf *msp.FabricMSPConfig) (*bccspmsp, error) {
	t.Helper()
	raw, err := proto.Marshal(conf)
	require.NoError(t, err)
	thisMSP, err := newBccspMsp(version, factory.GetDefault())
	require.NoError(t, err)
	return thisMSP.(*bccspmsp), thisMSP.Setup(&msp.MSPConfig{Type: int32(FABRIC), Config: raw})
}

func newTestCA(t *testing.T) tlsgen.CA {
	t.Helper()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	return ca
}

// issueCert returns a PEM leaf certificate for pub, signed by ca and carrying the given OUs.
func issueCert(t *testing.T, ca tlsgen.CA, pub crypto.PublicKey, ous ...string) []byte {
	t.Helper()
	block, _ := pem.Decode(ca.CertBytes())
	parent, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "leaf", OrganizationalUnit: ous},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, parent, pub, ca.Signer())
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
}

// newLeafIdentity returns a fresh, not yet validated identity issued by ca.
func newLeafIdentity(t *testing.T, thisMSP *bccspmsp, ca tlsgen.CA, ous ...string) *identity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id, err := thisMSP.deserializeIdentityInternal(issueCert(t, ca, &key.PublicKey, ous...))
	require.NoError(t, err)
	return id.(*identity)
}

func allNodeOUs() *msp.FabricNodeOUs {
	return &msp.FabricNodeOUs{
		Enable:              true,
		ClientOuIdentifier:  &msp.FabricOUIdentifier{OrganizationalUnitIdentifier: "client"},
		PeerOuIdentifier:    &msp.FabricOUIdentifier{OrganizationalUnitIdentifier: "peer"},
		AdminOuIdentifier:   &msp.FabricOUIdentifier{OrganizationalUnitIdentifier: "admin"},
		OrdererOuIdentifier: &msp.FabricOUIdentifier{OrganizationalUnitIdentifier: "orderer"},
	}
}

func TestMSPSetupInvalidConfig(t *testing.T) { //nolint:paralleltest
	thisMSP, err := newBccspMsp(MSPv1_0, factory.GetDefault())
	require.NoError(t, err)
	require.EqualError(t, thisMSP.Setup(nil), "Setup error: nil conf reference")
	require.ErrorContains(t, thisMSP.Setup(&msp.MSPConfig{Config: []byte{0xff}}), "failed unmarshalling fabric msp config")
}

func TestMSPSetupStageErrors(t *testing.T) { //nolint:paralleltest
	base := &msp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(conf.Config, base))
	foreignOU := &msp.FabricOUIdentifier{OrganizationalUnitIdentifier: "COP", Certificate: []byte(caCert)}
	withForeignNodeOU := func(set func(*msp.FabricNodeOUs)) func(*msp.FabricMSPConfig) {
		return func(c *msp.FabricMSPConfig) {
			c.FabricNodeOus = allNodeOUs()
			set(c.FabricNodeOus)
		}
	}
	bothVersions := []MSPVersion{MSPv1_1, MSPv1_4_3}

	for _, tc := range []struct { //nolint:paralleltest
		name     string
		versions []MSPVersion
		mutate   func(*msp.FabricMSPConfig)
		msg      string
	}{
		{"no root certs", bothVersions, func(c *msp.FabricMSPConfig) { c.RootCerts = nil }, "expected at least one CA certificate"},
		{"bad intermediate cert", bothVersions, func(c *msp.FabricMSPConfig) { c.IntermediateCerts = [][]byte{[]byte("garbage")} }, "getCertFromPem error"},
		{"bad admin cert", bothVersions, func(c *msp.FabricMSPConfig) { c.Admins = [][]byte{[]byte("garbage")} }, "getCertFromPem error"},
		{"bad CRL", bothVersions, func(c *msp.FabricMSPConfig) { c.RevocationList = [][]byte{[]byte("garbage")} }, "could not parse RevocationList"},
		{"bad signer", bothVersions, func(c *msp.FabricMSPConfig) {
			c.SigningIdentity = &msp.SigningIdentityInfo{PublicSigner: []byte("garbage")}
		}, "getCertFromPem error"},
		{"non-CA TLS root", bothVersions, func(c *msp.FabricMSPConfig) { c.TlsRootCerts = [][]byte{[]byte(nonCACert)} }, "CA Certificate did not have the CA attribute"},
		{"OU certificate not in CAs", bothVersions, func(c *msp.FabricMSPConfig) {
			c.OrganizationalUnitIdentifiers = []*msp.FabricOUIdentifier{foreignOU}
		}, "failed adding OU"},
		{"client node OU certificate not in CAs", bothVersions, withForeignNodeOU(func(n *msp.FabricNodeOUs) { n.ClientOuIdentifier = foreignOU }), "failed adding OU"},
		{"peer node OU certificate not in CAs", bothVersions, withForeignNodeOU(func(n *msp.FabricNodeOUs) { n.PeerOuIdentifier = foreignOU }), "failed adding OU"},
		{"admin node OU certificate not in CAs", []MSPVersion{MSPv1_4_3}, withForeignNodeOU(func(n *msp.FabricNodeOUs) { n.AdminOuIdentifier = foreignOU }), "failed adding OU"},
		{"orderer node OU certificate not in CAs", []MSPVersion{MSPv1_4_3}, withForeignNodeOU(func(n *msp.FabricNodeOUs) { n.OrdererOuIdentifier = foreignOU }), "failed adding OU"},
		{"no client node OU", []MSPVersion{MSPv1_1}, withForeignNodeOU(func(n *msp.FabricNodeOUs) { n.ClientOuIdentifier = nil }), "ClientOU must be different from nil"},
		{"no peer node OU", []MSPVersion{MSPv1_1}, withForeignNodeOU(func(n *msp.FabricNodeOUs) { n.PeerOuIdentifier = nil }), "PeerOU must be different from nil"},
		{"no admins and no admin node OU", []MSPVersion{MSPv1_4_3}, func(c *msp.FabricMSPConfig) { c.Admins = nil }, "administrators must be declared when no admin ou classification is set"},
	} {
		for _, version := range tc.versions {
			t.Run(fmt.Sprintf("%s/v%d", tc.name, version), func(t *testing.T) { //nolint:paralleltest
				c := proto.CloneOf(base)
				tc.mutate(c)
				_, err := setupFabricMSP(t, version, c)
				require.ErrorContains(t, err, tc.msg)
			})
		}
	}
}

func TestSetupOUsIgnoresDuplicates(t *testing.T) { //nolint:paralleltest
	c := &msp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(conf.Config, c))
	ou := &msp.FabricOUIdentifier{OrganizationalUnitIdentifier: "COP", Certificate: c.RootCerts[0]}
	c.OrganizationalUnitIdentifiers = []*msp.FabricOUIdentifier{ou, ou}

	thisMSP, err := setupFabricMSP(t, MSPv1_0, c)
	require.NoError(t, err)
	require.Len(t, thisMSP.ouIdentifiers["COP"], 1)
}

func TestValidateIdentityOUsV1(t *testing.T) { //nolint:paralleltest
	ca := newTestCA(t)
	thisMSP, err := setupFabricMSP(t, MSPv1_0, &msp.FabricMSPConfig{
		Name:                          "TestOrg",
		RootCerts:                     [][]byte{ca.CertBytes()},
		OrganizationalUnitIdentifiers: []*msp.FabricOUIdentifier{{OrganizationalUnitIdentifier: "member", Certificate: ca.CertBytes()}},
	})
	require.NoError(t, err)

	require.NoError(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca, "member")))
	require.ErrorContains(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca)), "the identity certificate does not contain an Organizational Unit (OU)")
	require.ErrorContains(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca, "other")), "none of the identity's organizational units")
}

func TestValidateIdentityNodeOUs(t *testing.T) { //nolint:paralleltest
	for _, version := range []MSPVersion{MSPv1_1, MSPv1_4_3} {
		ca := newTestCA(t)
		thisMSP, err := setupFabricMSP(t, version, &msp.FabricMSPConfig{
			Name:          "TestOrg",
			RootCerts:     [][]byte{ca.CertBytes()},
			FabricNodeOus: allNodeOUs(),
		})
		require.NoError(t, err)

		require.NoError(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca, "client")))
		require.ErrorContains(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca)), "the identity does not have an OU that resolves to")
		require.ErrorContains(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca, "client", "peer")), "not a combination of them")

		thisMSP.clientOU.CertifiersIdentifier = []byte("another chain")
		require.ErrorContains(t, thisMSP.Validate(newLeafIdentity(t, thisMSP, ca, "client")), "certifiersIdentifier does not match")
	}
}

func TestHasOURole(t *testing.T) { //nolint:paralleltest
	require.EqualError(t, localMsp.(*bccspmsp).hasOURole(getIdentity(t, signcerts), msp.MSPRole_CLIENT), "nodeOUs not activated. Cannot tell apart identities")

	ca := newTestCA(t)
	v142, err := setupFabricMSP(t, MSPv1_4_3, &msp.FabricMSPConfig{Name: "TestOrg", RootCerts: [][]byte{ca.CertBytes()}, FabricNodeOus: allNodeOUs()})
	require.NoError(t, err)
	client := newLeafIdentity(t, v142, ca, "client")
	require.NoError(t, v142.hasOURole(client, msp.MSPRole_CLIENT))
	require.EqualError(t, v142.hasOURole(&idemixIdentityWrapper{}, msp.MSPRole_CLIENT), "Identity type not recognized")
	require.EqualError(t, v142.hasOURoleInternal(client, msp.MSPRole_MEMBER), "Invalid MSPRoleType. It must be CLIENT, PEER, ADMIN or ORDERER")

	nodeOUs := allNodeOUs()
	nodeOUs.AdminOuIdentifier = nil
	v142, err = setupFabricMSP(t, MSPv1_4_3, &msp.FabricMSPConfig{Name: "TestOrg", RootCerts: [][]byte{ca.CertBytes()}, FabricNodeOus: nodeOUs, Admins: [][]byte{issueCert(t, ca, client.cert.PublicKey, "client")}})
	require.NoError(t, err)
	require.ErrorContains(t, v142.hasOURoleInternal(client, msp.MSPRole_ADMIN), "cannot test for classification")
}

func principal(t *testing.T, classification msp.MSPPrincipal_Classification, m proto.Message) *msp.MSPPrincipal {
	t.Helper()
	raw, err := proto.Marshal(m)
	require.NoError(t, err)
	return &msp.MSPPrincipal{PrincipalClassification: classification, Principal: raw}
}

func TestSatisfiesPrincipalInternalV13(t *testing.T) { //nolint:paralleltest
	id := getIdentity(t, signcerts)
	anonymity := func(typ msp.MSPIdentityAnonymity_MSPIdentityAnonymityType) *msp.MSPPrincipal {
		return principal(t, msp.MSPPrincipal_ANONYMITY, &msp.MSPIdentityAnonymity{AnonymityType: typ})
	}
	garbage := func(classification msp.MSPPrincipal_Classification) *msp.MSPPrincipal {
		return &msp.MSPPrincipal{PrincipalClassification: classification, Principal: []byte{0xff}}
	}

	for _, tc := range []struct { //nolint:paralleltest
		name      string
		principal *msp.MSPPrincipal
		msg       string
	}{
		{"combined", &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_COMBINED}, "shall not be called with a CombinedPrincipal"},
		{"invalid anonymity", garbage(msp.MSPPrincipal_ANONYMITY), "could not unmarshal MSPIdentityAnonymity from principal"},
		{"anonymous", anonymity(msp.MSPIdentityAnonymity_ANONYMOUS), "X.509 MSP does not support anonymous identities"},
		{"nominal", anonymity(msp.MSPIdentityAnonymity_NOMINAL), ""},
		{"unknown anonymity", anonymity(42), "Unknown principal anonymity type: 42"},
		{"invalid role", garbage(msp.MSPPrincipal_ROLE), "could not unmarshal MSPRole from principal"},
		{"invalid OU", garbage(msp.MSPPrincipal_ORGANIZATION_UNIT), "could not unmarshal OrganizationUnit from principal"},
		{"OU of another MSP", principal(t, msp.MSPPrincipal_ORGANIZATION_UNIT, &msp.OrganizationUnit{MspIdentifier: "OtherOrg"}), "the identity is a member of a different MSP"},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest
			err := localMspV13.(*bccspmsp).satisfiesPrincipalInternalV13(id, tc.principal)
			if tc.msg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestSatisfiesPrincipalInternalV142(t *testing.T) { //nolint:paralleltest
	thisMSP := getLocalMSPWithVersion(t, "testdata/nodeouadmin", MSPv1_4_3).(*bccspmsp)
	signer, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	peer := signer.GetPublicVersion()
	role := func(r msp.MSPRole_MSPRoleType, mspID string) *msp.MSPPrincipal {
		return principal(t, msp.MSPPrincipal_ROLE, &msp.MSPRole{Role: r, MspIdentifier: mspID})
	}

	for _, tc := range []struct { //nolint:paralleltest
		name      string
		id        Identity
		principal *msp.MSPPrincipal
		msg       string
	}{
		{"not an X.509 identity", &idemixIdentityWrapper{}, role(msp.MSPRole_MEMBER, thisMSP.name), "invalid identity type, expected *identity"},
		{"invalid role", peer, &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_ROLE, Principal: []byte{0xff}}, "could not unmarshal MSPRole from principal"},
		{"role of another MSP", peer, role(msp.MSPRole_PEER, "OtherOrg"), "the identity is a member of a different MSP"},
		{"peer is not an admin", peer, role(msp.MSPRole_ADMIN, thisMSP.name), "The identity is not an admin under this MSP"},
		{"peer is not an orderer", peer, role(msp.MSPRole_ORDERER, thisMSP.name), "The identity is not a [ORDERER] under this MSP"},
		{"peer is a peer", peer, role(msp.MSPRole_PEER, thisMSP.name), ""},
		{"nominal", peer, principal(t, msp.MSPPrincipal_ANONYMITY, &msp.MSPIdentityAnonymity{AnonymityType: msp.MSPIdentityAnonymity_NOMINAL}), ""},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest
			err := thisMSP.satisfiesPrincipalInternalV142(tc.id, tc.principal)
			if tc.msg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestIsWellFormedCertificates(t *testing.T) { //nolint:paralleltest
	ca := newTestCA(t)
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	block, _ := pem.Decode(issueCert(t, ca, &ecKey.PublicKey))
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	badSig, err := certFromX509Cert(x509Cert)
	require.NoError(t, err)
	badSig.Raw = nil
	badSig.SignatureValue = asn1.BitString{Bytes: []byte{1, 2, 3}, BitLength: 24}

	for _, tc := range []struct { //nolint:paralleltest
		name    string
		idBytes []byte
		msg     string
	}{
		{"not PEM", []byte("garbage"), "PEM decoding resulted in an empty block"},
		{"invalid DER", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage")}), "x509"},
		{"non-ECDSA certificate", issueCert(t, ca, edPub), ""},
		{"ECDSA signature not DER encoded", []byte(badSig.String()), "asn1"},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest
			err := localMsp.IsWellFormed(&msp.SerializedIdentity{Mspid: "SampleOrg", IdBytes: tc.idBytes})
			if tc.msg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestCertificationChainErrors(t *testing.T) { //nolint:paralleltest
	ca := newTestCA(t)
	thisMSP, err := setupFabricMSP(t, MSPv1_0, &msp.FabricMSPConfig{Name: "TestOrg", RootCerts: [][]byte{ca.CertBytes()}})
	require.NoError(t, err)
	root := thisMSP.rootCerts[0].(*identity)
	leaf := newLeafIdentity(t, thisMSP, ca)

	_, err = thisMSP.getCertificationChain(&idemixIdentityWrapper{})
	require.EqualError(t, err, "identity type not recognized")
	_, err = thisMSP.getCertificationChainForBCCSPIdentity(nil)
	require.EqualError(t, err, "invalid bccsp identity. Must be different from nil")
	_, err = (&bccspmsp{}).getCertificationChainForBCCSPIdentity(leaf)
	require.EqualError(t, err, "Invalid msp instance")
	_, err = thisMSP.getCertificationChainForBCCSPIdentity(root)
	require.ErrorContains(t, err, "Certificate Authority equals true cannot be used as an identity")
	_, err = (&bccspmsp{}).getUniqueValidationChain(leaf.cert, x509.VerifyOptions{})
	require.EqualError(t, err, "the supplied identity has no verify options")
	_, err = thisMSP.getValidationChain(root.cert, false)
	require.EqualError(t, err, "expected a chain of length at least 2, got 1")

	require.EqualError(t, thisMSP.validateCAIdentity(leaf), "Only CA identities can be validated")
	require.EqualError(t, thisMSP.validateTLSCAIdentity(leaf.cert, nil), "Only CA identities can be validated")

	_, err = getAuthorityKeyIdentifierFromCrl(&pkix.CertificateList{})
	require.EqualError(t, err, "authorityKeyIdentifier not found in certificate")
}

func TestCertToPEM(t *testing.T) { //nolint:paralleltest
	cert := getIdentity(t, signcerts).(*identity).cert
	block, rest := pem.Decode([]byte(certToPEM(cert)))
	require.NotNil(t, block)
	require.Empty(t, rest)
	require.Equal(t, cert.Raw, block.Bytes)

	require.Empty(t, certToPEM(&x509.Certificate{Raw: []byte("garbage")}))
}
