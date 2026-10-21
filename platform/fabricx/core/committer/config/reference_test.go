/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/configtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
)

// TestReferenceConfiguration checks the Fabric-x sections of the Fabric network block in
// docs/configuration.md against the Fabric-x platform code that reads them.
func TestReferenceConfiguration(t *testing.T) {
	t.Parallel()
	r := configtest.Load(t)
	const network = configtest.Network

	r.Decode(network+".queryService", &config.Config{}, "endpoints")
	r.DecodeEndpoints(network + ".queryService.endpoints")

	// Read by platform/fabricx/core/channel/config.
	r.Read(
		network+".configMonitor.pollInterval",
		network+".configMonitor.maxRetries",
		network+".configMonitor.initialRetryDelay",
		network+".configMonitor.maxRetryDelay",
	)

	r.RequireComplete(network+".queryService", network+".configMonitor")
}
