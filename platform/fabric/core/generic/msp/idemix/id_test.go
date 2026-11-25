/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"os"
	"testing"
	"time"

	bccsp "github.com/IBM/idemix/bccsp/types"
	imsp "github.com/IBM/idemix/msp"
	math "github.com/IBM/mathlib"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

func TestMSPIdentityMethods(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl

	id := &idemix.MSPIdentity{
		ID: &msp.IdentityIdentifier{Mspid: "test-msp"},
		OU: &m.OrganizationUnit{
			OrganizationalUnitIdentifier: "test-ou",
			CertifiersIdentifier:         []byte("cert"),
		},
		Role: &m.MSPRole{
			Role: m.MSPRole_ADMIN,
		},
		Idemix: &idemix.Idemix{Name: "test-msp"},
	}

	require.True(t, id.Anonymous())

	expiresAt := id.ExpiresAt()
	require.Equal(t, time.Time{}, expiresAt)

	identifier := id.GetIdentifier()
	require.Equal(t, "test-msp", identifier.Mspid)
	require.Equal(t, "test-msp", id.GetMSPIdentifier())

	// GetOrganizationalUnits returns nil when the issuer public key cannot be serialized
	idBadIPK := &idemix.MSPIdentity{
		OU:     &m.OrganizationUnit{OrganizationalUnitIdentifier: "test-ou"},
		Idemix: &idemix.Idemix{Name: "test-msp", IssuerPublicKey: failingKey{}},
	}
	require.Nil(t, idBadIPK.GetOrganizationalUnits())

	// GetOrganizationalUnits returns OU with certifiers identifier equal to IPK bytes
	ipkBytes, err := os.ReadFile("./testdata/idemix/msp/IssuerPublicKey")
	require.NoError(t, err)
	cryptoProvider, err := idemix.NewBCCSP(math.FP256BN_AMCL)
	require.NoError(t, err)
	ipkKey, err := cryptoProvider.KeyImport(ipkBytes, &bccsp.IdemixIssuerPublicKeyImportOpts{
		Temporary: true,
		AttributeNames: []string{
			imsp.AttributeNameOU,
			imsp.AttributeNameRole,
			imsp.AttributeNameEnrollmentId,
			imsp.AttributeNameRevocationHandle,
		},
	})
	require.NoError(t, err)

	idWithIPK := &idemix.MSPIdentity{
		ID: &msp.IdentityIdentifier{Mspid: "test-msp"},
		OU: &m.OrganizationUnit{
			OrganizationalUnitIdentifier: "test-ou",
		},
		Idemix: &idemix.Idemix{
			Name:            "test-msp",
			IssuerPublicKey: ipkKey,
		},
	}
	ous := idWithIPK.GetOrganizationalUnits()
	require.Len(t, ous, 1)
	require.Equal(t, "test-ou", ous[0].OrganizationalUnitIdentifier)
	require.Equal(t, ipkBytes, ous[0].CertifiersIdentifier)

	err = id.SatisfiesPrincipal(nil)
	require.ErrorContains(t, err, "not supported")

	sigID := &idemix.MSPSigningIdentity{
		MSPIdentity: id,
	}
	publicVersion := sigID.GetPublicVersion()
	require.Equal(t, id, publicVersion)

	// Sign error path (nil Idemix/Csp or UserKey)
	sigIDWithCSP := &idemix.MSPSigningIdentity{
		MSPIdentity: &idemix.MSPIdentity{
			Idemix: &idemix.Idemix{Csp: cryptoProvider},
		},
	}
	_, err = sigIDWithCSP.Sign([]byte("msg"))
	require.ErrorContains(t, err, "invalid key: it must not be nil")
}

func TestAuditInfo_Methods(t *testing.T) {
	t.Parallel()

	ai := &idemix.AuditInfo{
		Attributes: [][]byte{
			[]byte("org-unit"),
			[]byte("1"),
			[]byte("alice-eid"),
			[]byte("alice-rh"),
		},
	}
	require.Equal(t, "alice-eid", ai.EnrollmentID())
	require.Equal(t, "alice-rh", ai.RevocationHandle())

	// Match unmarshal errors
	err := ai.Match([]byte("not-a-proto"))
	require.ErrorContains(t, err, "failed to unmarshal to msp.SerializedIdentity{}")

	si := &m.SerializedIdentity{
		Mspid:   "idemix",
		IdBytes: []byte("invalid-idemix-bytes"),
	}
	raw, err := proto.Marshal(si)
	require.NoError(t, err)
	err = ai.Match(raw)
	require.ErrorContains(t, err, "could not deserialize a SerializedIdemixIdentity")
}

func TestNymSignatureVerifier_Verify(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	csp, err := idemix.NewBCCSP(math.BN254)
	require.NoError(t, err)

	v := &idemix.NymSignatureVerifier{
		CSP: csp,
	}
	err = v.Verify(nil, nil)
	require.Error(t, err)
}

// failingKey is a bccsp.Key whose serialization always fails.
type failingKey struct{}

func (failingKey) Bytes() ([]byte, error)        { return nil, errors.New("bytes failed") }
func (failingKey) SKI() []byte                   { return nil }
func (failingKey) Symmetric() bool               { return false }
func (failingKey) Private() bool                 { return false }
func (failingKey) PublicKey() (bccsp.Key, error) { return nil, errors.New("no public key") }
