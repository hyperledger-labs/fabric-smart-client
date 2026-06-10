/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"testing"
	"time"

	math "github.com/IBM/mathlib"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

func TestMSPIdentityMethods(t *testing.T) { //nolint:paralleltest

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

	err := id.SatisfiesPrincipal(nil)
	require.ErrorContains(t, err, "not supported")

	sigId := &idemix.MSPSigningIdentity{
		MSPIdentity: id,
	}
	publicVersion := sigId.GetPublicVersion()
	require.Equal(t, id, publicVersion)
}

func TestNymSignatureVerifier_Verify(t *testing.T) { //nolint:paralleltest
	csp, err := idemix.NewBCCSP(math.BN254)
	require.NoError(t, err)

	v := &idemix.NymSignatureVerifier{
		CSP: csp,
	}
	err = v.Verify(nil, nil)
	require.Error(t, err)
}
