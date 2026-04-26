/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fxconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamespaceCommonEnvExportsGlobalTLSCompatVars(t *testing.T) {
	t.Parallel()

	env := (&NamespaceCommon{
		MSPConfig: MSPConfig{
			LocalMspID: "Org1MSP",
			ConfigPath: "/tmp/msp",
		},
		Channel: "testchannel",
		OrdererConfig: OrdererConfig{
			Address: "127.0.0.1:7050",
			TLSConfig: TLSConfig{
				Enabled:        true,
				RootCerts:      []string{"/tmp/orderer-ca.pem"},
				ClientKeyPath:  "/tmp/client.key",
				ClientCertPath: "/tmp/client.crt",
			},
		},
		NotificationsConfig: NotificationsConfig{
			Address: "127.0.0.1:5414",
		},
	}).Env()

	require.Contains(t, env, "FXCONFIG_TLS_ENABLED=true")
	require.Contains(t, env, "FXCONFIG_TLS_ROOTCERTS=/tmp/orderer-ca.pem")
	require.Contains(t, env, "FXCONFIG_TLS_CLIENTKEY=/tmp/client.key")
	require.Contains(t, env, "FXCONFIG_TLS_CLIENTCERT=/tmp/client.crt")
}

func TestListNamespacesEnvExportsGlobalTLSCompatVars(t *testing.T) {
	t.Parallel()

	env := (&ListNamespaces{
		QueryConfig: QueryConfig{
			Address: "127.0.0.1:7001",
			TLSConfig: TLSConfig{
				Enabled:   true,
				RootCerts: []string{"/tmp/query-ca.pem"},
			},
		},
	}).Env()

	require.Contains(t, env, "FXCONFIG_TLS_ENABLED=true")
	require.Contains(t, env, "FXCONFIG_TLS_ROOTCERTS=/tmp/query-ca.pem")
}
