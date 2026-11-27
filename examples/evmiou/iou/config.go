/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Config mirrors the "evm" section of a node's core.yaml, rendered by
// examples/evmiou/nwo/evm.RenderExtension.
type Config struct {
	Endpoint string         `yaml:"endpoint"`
	ChainID  int64          `yaml:"chainID"`
	Contract string         `yaml:"contract"`
	Key      string         `yaml:"key"`
	Gas      GasConfig      `yaml:"gas"`
	Finality FinalityConfig `yaml:"finality"`
}

// GasConfig is the "evm.gas" section: the multiplier chain.Submitter applies to an estimated gas
// limit.
type GasConfig struct {
	Multiplier float64 `yaml:"multiplier"`
}

// FinalityConfig is the "evm.finality" section: the poll interval and timeout chain.WaitReceipt
// uses while waiting for a transaction's receipt.
type FinalityConfig struct {
	PollInterval time.Duration `yaml:"pollInterval"`
	Timeout      time.Duration `yaml:"timeout"`
}

// LoadKey reads a secp256k1 private key from a file holding its hex-encoded scalar, optionally
// 0x-prefixed and with surrounding whitespace, which is the format foundry and the common EVM dev
// tooling emit.
//
// Copied from Panurus x/token/services/network/evm/keystore.go (LoadKey, decodeKeyScalar) at
// commit f144b2b7f, adapted to this package's own eip712 decoder.
func LoadKey(path string) (*secp256k1.PrivateKey, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, errors.Wrapf(err, "iou: failed to read the key at [%s]", path)
	}

	key, err := ParseKeyHex(string(raw))
	if err != nil {
		return nil, errors.Wrapf(err, "iou: the key at [%s] is invalid", path)
	}

	return key, nil
}

// ParseKeyHex parses a secp256k1 private key from its hex-encoded scalar, optionally 0x-prefixed
// and with surrounding whitespace, which is the format foundry and the common EVM dev tooling
// emit.
func ParseKeyHex(s string) (*secp256k1.PrivateKey, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")

	scalar, err := hex.DecodeString(s)
	if err != nil {
		return nil, errors.Wrap(err, "iou: the key must be hex encoded")
	}
	if len(scalar) != eip712.PrivateKeyLength {
		return nil, errors.Errorf("iou: the key must be %d bytes, got %d", eip712.PrivateKeyLength, len(scalar))
	}

	key, err := eip712.DecodePrivateKeyScalar(scalar)
	if err != nil {
		return nil, errors.Wrap(err, "iou: the key is invalid")
	}

	return key, nil
}
