/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/stretchr/testify/require"
)

func TestInfoIdemix(t *testing.T) {
	t.Parallel()
	driver := mem.NewDriver()
	persistence, err := driver.NewKVS("")
	require.NoError(t, err)
	auditInfo, err := driver.NewAuditInfo("")
	require.NoError(t, err)
	signerInfo, err := driver.NewSignerInfo("")
	require.NoError(t, err)
	kvss, err := kvs.New(persistence, "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), auditInfo, signerInfo)

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, _, err := p.Identity(nil)
	require.NoError(t, err)

	s := Info(id, nil)
	require.True(t, strings.HasPrefix(s, "MSP.Idemix: []"))
	require.True(t, strings.HasSuffix(s, "[idemix][idemixorg.example.com][ADMIN]"))
}

func TestInfoX509(t *testing.T) {
	t.Parallel()
	p, err := x509.NewProvider("./testdata/x509", "", "apple", nil)
	require.NoError(t, err)
	id, _, err := p.Identity(nil)
	require.NoError(t, err)

	s := Info(id, nil)
	require.Equal(t, "MSP.x509: [f+hVlmGaPejN2G0XDcESSMX2ol29WPcPQ+Fp3lOARBQ=][apple][auditor.org1.example.com]", s)
}
