/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	configservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	koanffile "github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Parallel()
	v := koanf.New(".")
	require.NoError(t, v.Load(koanffile.Provider("./testdata/core.yaml"), configservice.LowercaseParser{Parser: koanfyaml.Parser()}))

	var value interface{}
	require.NoError(t, v.Unmarshal("fabric", &value))

	var network config.Network
	require.NoError(t, configservice.EnhancedExactUnmarshal(v, "fabric.default", &network))

	b, err := x509.ToBCCSPOpts(network.MSPs[0].Opts[x509.BCCSPOptField])
	require.NoError(t, err)
	assert.NotNil(t, b.SW)
	assert.Equal(t, "SHA2", b.SW.Hash)
	assert.NotNil(t, b.PKCS11)
	assert.Equal(t, 256, b.PKCS11.Security)
	assert.Equal(t, "ForFSC", b.PKCS11.Label)
	assert.Equal(t, "98765432", b.PKCS11.Pin)
}
