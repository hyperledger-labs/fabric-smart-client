/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	viperutil "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config/viper"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	v := viper.New()
	v.SetConfigName("core")
	v.AddConfigPath("./testdata")
	replacer := strings.NewReplacer(".", "_")
	v.SetEnvKeyReplacer(replacer)
	assert.NoError(t, v.ReadInConfig())

	var value interface{}
	assert.NoError(t, v.UnmarshalKey("fabric", &value))

	var network config.Network
	assert.NoError(t, viperutil.EnhancedExactUnmarshal(v, "fabric.default", &network))

	b, err := x509.ToBCCSPOpts(network.MSPs[0].Opts[x509.BCCSPOptField])
	assert.NoError(t, err)
	assert.NotNil(t, b.SW)
	assert.Equal(t, "SHA2", b.SW.Hash)
	assert.NotNil(t, b.PKCS11)
	assert.Equal(t, 256, b.PKCS11.Security)
	assert.Equal(t, "someLabel", b.PKCS11.Label)
	assert.Equal(t, "98765432", b.PKCS11.Pin)
}
