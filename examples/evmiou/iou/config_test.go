/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client/mock"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// keyScalarHex is a 32-byte private-key scalar (low byte 1), hex encoded with no prefix.
const keyScalarHex = "0000000000000000000000000000000000000000000000000000000000000001"

func writeKeyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// TestLoadKeyFormats checks that the same scalar, written plain, 0x-prefixed with a trailing
// newline, and surrounded by whitespace, all load to the same key.
func TestLoadKeyFormats(t *testing.T) {
	t.Parallel()

	plain, err := LoadKey(writeKeyFile(t, keyScalarHex))
	require.NoError(t, err)
	prefixed, err := LoadKey(writeKeyFile(t, "0x"+keyScalarHex+"\n"))
	require.NoError(t, err)
	padded, err := LoadKey(writeKeyFile(t, "  "+keyScalarHex+"  "))
	require.NoError(t, err)

	want := eip712.PubKeyToAddress(plain.PubKey())
	assert.Equal(t, want, eip712.PubKeyToAddress(prefixed.PubKey()))
	assert.Equal(t, want, eip712.PubKeyToAddress(padded.PubKey()))
}

// TestLoadKeyNotHex checks that a non-hex key file is an error.
func TestLoadKeyNotHex(t *testing.T) {
	t.Parallel()

	_, err := LoadKey(writeKeyFile(t, "not hex"))
	require.Error(t, err)
}

// TestParseKeyHex checks that a 0x-prefixed scalar (as EVM_FUNDER_KEY and the README's documented
// paste both give it) parses to the same key as the bare scalar, and that a scalar of the wrong
// length is rejected rather than silently truncated.
func TestParseKeyHex(t *testing.T) {
	t.Parallel()

	plain, err := ParseKeyHex(keyScalarHex)
	require.NoError(t, err)
	prefixed, err := ParseKeyHex("0x" + keyScalarHex)
	require.NoError(t, err)
	assert.Equal(t, eip712.PubKeyToAddress(plain.PubKey()), eip712.PubKeyToAddress(prefixed.PubKey()))

	_, err = ParseKeyHex(keyScalarHex + "ff")
	require.Error(t, err)
}

// TestNewServiceChainIDMismatch checks that NewService fails, naming both chain ids, when the
// node's chain id does not match the configuration.
func TestNewServiceChainIDMismatch(t *testing.T) {
	t.Parallel()

	fake := &mock.EVMClient{}
	fake.ChainIDReturns(big.NewInt(1337), nil)

	cfg := Config{ChainID: 298}
	_, err := buildService(context.Background(), cfg, fake)

	require.Error(t, err)
	require.ErrorContains(t, err, "1337")
	require.ErrorContains(t, err, "298")
}

// TestConfigUnmarshalDurations checks that FSC's config provider decodes the "evm" section the
// way examples/evmiou/nwo/evm.RenderExtension renders it, in particular the finality poll
// interval/timeout, rendered as Go duration strings ("1s", "1m0s"), into time.Duration.
func TestConfigUnmarshalDurations(t *testing.T) {
	t.Parallel()

	raw := []byte(`evm:
  endpoint: http://127.0.0.1:8545
  chainID: 298
  contract: "0x1111111111111111111111111111111111111111"
  key: /tmp/key
  gas:
    multiplier: 1.2
  finality:
    pollInterval: "1s"
    timeout: "1m0s"
`)
	p, err := (&config.Provider{}).ProvideFromRaw(raw)
	require.NoError(t, err)

	var cfg Config
	require.NoError(t, p.UnmarshalKey("evm", &cfg))

	assert.Equal(t, "http://127.0.0.1:8545", cfg.Endpoint)
	assert.Equal(t, int64(298), cfg.ChainID)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", cfg.Contract)
	assert.Equal(t, "/tmp/key", cfg.Key)
	assert.InDelta(t, 1.2, cfg.Gas.Multiplier, 0.0001)
	assert.Equal(t, time.Second, cfg.Finality.PollInterval)
	assert.Equal(t, time.Minute, cfg.Finality.Timeout)
}
