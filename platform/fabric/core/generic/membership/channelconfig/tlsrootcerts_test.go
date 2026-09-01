/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/fake"
)

// TestApplicationOrgTLSCertsRootsFirstThenIntermediates pins the ordering that
// TLSRootCertsByMSPID (platform/fabric/core/generic/membership) must preserve
// when it reads TLS certificates out of an application organization's MSP:
// root certificates first, intermediate certificates second.
func TestApplicationOrgTLSCertsRootsFirstThenIntermediates(t *testing.T) {
	t.Parallel()

	m := &fake.MSP{}
	m.On("GetTLSRootCerts").Return([][]byte{[]byte("root")})
	m.On("GetTLSIntermediateCerts").Return([][]byte{[]byte("inter")})

	org := &ApplicationOrgConfig{
		name: "Org1",
		OrganizationConfig: &OrganizationConfig{
			name:  "Org1",
			mspID: "Org1MSP",
			msp:   m,
		},
	}

	ac := &ApplicationConfig{
		applicationOrgs: map[string]ApplicationOrg{
			"Org1": org,
		},
	}

	var found ApplicationOrg
	for _, o := range ac.Organizations() {
		if o.MSPID() == "Org1MSP" {
			found = o
			break
		}
	}
	require.NotNil(t, found, "expected to find an organization with MSP ID [Org1MSP]")

	msp := found.MSP()
	var tlsRootCerts [][]byte
	tlsRootCerts = append(tlsRootCerts, msp.GetTLSRootCerts()...)
	tlsRootCerts = append(tlsRootCerts, msp.GetTLSIntermediateCerts()...)

	require.Equal(t, [][]byte{[]byte("root"), []byte("inter")}, tlsRootCerts,
		"roots must come first, intermediates second")
}
