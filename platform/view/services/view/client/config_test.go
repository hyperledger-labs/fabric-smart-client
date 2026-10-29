/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
)

func TestValidateClientConfig(t *testing.T) {
	t.Parallel()

	validConfig := func() client.Config {
		return client.Config{
			ConnectionConfig: &grpc.ConnectionConfig{
				Address: "127.0.0.1:0",
				TLS: grpc.SecureOptions{
					UseTLS:        true,
					ServerRootCAs: [][]byte{[]byte("cert")},
				},
			},
		}
	}

	t.Run("returns no error for a valid config", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, client.ValidateClientConfig(validConfig()))
	})

	t.Run("returns missing fsc address error", func(t *testing.T) {
		t.Parallel()
		config := validConfig()
		config.ConnectionConfig.Address = ""
		require.EqualError(t, client.ValidateClientConfig(config), "missing fsc peer address")
	})

	// A TLS-enabled connection with no trust anchor in the resolved pool can never verify
	// the server, however the anchors were configured.
	t.Run("returns a missing root certificates error when TLS is enabled", func(t *testing.T) {
		t.Parallel()
		config := validConfig()
		config.ConnectionConfig.TLS.ServerRootCAs = nil
		require.EqualError(t, client.ValidateClientConfig(config), "missing fsc peer TLS root certificates")
	})
}
