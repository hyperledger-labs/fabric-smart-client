/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	viperutil "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config/viper"
)

func TestLoad(t *testing.T) {
	v := viper.New()
	v.SetConfigName("core")
	v.AddConfigPath("./testdata")
	replacer := strings.NewReplacer(".", "_")
	v.SetEnvKeyReplacer(replacer)
	require.NoError(t, v.ReadInConfig())

	var value interface{}
	require.NoError(t, v.UnmarshalKey("fabric", &value))

	var network Network
	require.NoError(t, viperutil.EnhancedExactUnmarshal(v, "fabric.default", &network))
}
