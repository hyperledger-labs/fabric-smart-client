/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"testing"

	"github.com/stretchr/testify/require"

	fabrictopology "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func TestTLSServerName(t *testing.T) {
	t.Parallel()

	orderer := &fabrictopology.Orderer{
		Name:         "orderer",
		Organization: "OrdererOrg",
	}
	org := &fabrictopology.Organization{
		Name:   "OrdererOrg",
		Domain: "example.com",
	}

	require.Equal(t, "orderer.example.com", tlsServerName(orderer, org))
}

func TestTLSServerNameWithoutDomain(t *testing.T) {
	t.Parallel()

	orderer := &fabrictopology.Orderer{Name: "orderer"}

	require.Empty(t, tlsServerName(orderer, nil))
	require.Empty(t, tlsServerName(orderer, &fabrictopology.Organization{}))
}
