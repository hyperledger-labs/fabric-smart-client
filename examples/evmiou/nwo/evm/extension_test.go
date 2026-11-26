/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
)

func TestRenderExtension(t *testing.T) {
	t.Parallel()

	addr, err := client.HexToAddress("0x00112233445566778899aabbccddeeff0011223a")
	require.NoError(t, err)

	cfg := NodeConfig{
		Endpoint:      "http://127.0.0.1:32769",
		ChainID:       1337,
		Contract:      addr,
		KeyPath:       "/tmp/evm/keys/node1.key",
		GasMultiplier: 1.2,
		PollInterval:  time.Second,
		Timeout:       time.Minute,
	}

	out, err := RenderExtension(cfg)
	require.NoError(t, err)

	var doc extensionDoc
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))

	require.Equal(t, cfg.Endpoint, doc.EVM.Endpoint)
	require.Equal(t, cfg.ChainID, doc.EVM.ChainID)
	require.Equal(t, cfg.Contract.Hex(), doc.EVM.Contract)
	require.Equal(t, cfg.KeyPath, doc.EVM.Key)
	require.InDelta(t, cfg.GasMultiplier, doc.EVM.Gas.Multiplier, 0)

	gotPoll, err := time.ParseDuration(doc.EVM.Finality.PollInterval)
	require.NoError(t, err)
	require.Equal(t, cfg.PollInterval, gotPoll)

	gotTimeout, err := time.ParseDuration(doc.EVM.Finality.Timeout)
	require.NoError(t, err)
	require.Equal(t, cfg.Timeout, gotTimeout)
}
