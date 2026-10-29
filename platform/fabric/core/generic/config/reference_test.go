/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/configtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
)

// TestReferenceConfiguration checks the fabric section of docs/configuration.md against the
// Fabric platform code that reads it. Fabric-x sections inside the network block are checked
// by the Fabric-x platform's reference test.
func TestReferenceConfiguration(t *testing.T) {
	t.Parallel()
	r := configtest.Load(t)
	const network = configtest.Network

	// fabric.enabled, and the network's default and driver, are read by platform/fabric/core.
	// default and driver decode into core.FSNConfig from the whole network block, which
	// holds every other network key too, so no strict decode can check them.
	r.Read(
		"fabric.enabled",
		network+".default",
		network+".driver",
		network+".mspCacheSize",
		network+".defaultMSP",
		network+".keepalive.connectionTimeout",
		network+".ordering.numRetries",
		network+".ordering.retryInterval",
		network+".ordering.connectionPoolSize",
		network+".vault.persistence",
		network+".vault.txidstore.cache.size",
	)

	r.Decode(network+".tls", &tlsconfig.ClientTLS{})
	r.Decode(network+".keepalive", &grpc.ClientKeepAliveConfig{}, "connectionTimeout")
	r.DecodeEndpoints(network + ".orderers")
	r.DecodeEndpoints(network + ".peers")
	r.DecodeEach(network+".channels", func() any { return &config.Channel{} })
	r.DecodeEach(network+".endpoint.resolvers", func() any { return &config.Resolver{} })
	r.DecodeEach(network+".msps", func() any { return &config.MSP{} })
	// MSP.Opts is a free-form map. The x509 loader reads only its bccsp entry, decoded into
	// BCCSP, so any other key under opts is an error here.
	for i, msp := range r.RawSubtrees(network + ".msps") {
		if opts, ok := msp["opts"].(map[string]any); ok {
			bccsp, ok := opts["bccsp"].(map[string]any)
			require.True(t, ok && len(opts) == 1, "%s.msps[%d].opts must hold only a BCCSP block", network, i)
			r.Check(fmt.Sprintf("%s.msps[%d].opts.bccsp", network, i), bccsp, &config.BCCSP{})
		}
	}

	r.Delegate(network+".queryService", network+".notificationService", network+".configMonitor")

	r.RequireComplete("fabric")
}
