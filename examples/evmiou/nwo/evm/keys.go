/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Adapted from Panurus (x/token/services/network/evm) at commit f144b2b7f.
package evm

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Besu's development network pre-funds a fixed set of accounts. The funder has to pay for gas,
// so GenerateArtifacts uses one of them in Besu mode rather than a freshly generated key that
// would hold no ether.
const (
	// DevFundedKey is Besu dev mode's first pre-funded account's private key, hex-encoded.
	DevFundedKey = "8f2a55949038a9610f50fb23b5883af3b4ecb3c3bb792cbcefbd1542c692be63"
)

// Identity is a generated secp256k1 EVM identity: the key, the address it derives, and the file
// its hex-encoded scalar was written to.
type Identity struct {
	Key     *secp256k1.PrivateKey
	Address client.Address
	Path    string
}

// GenerateKey creates a fresh secp256k1 key and writes its hex-encoded scalar to
// <dir>/<name>.key, creating dir if needed.
func GenerateKey(dir, name string) (Identity, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Identity{}, errors.Wrapf(err, "evm nwo: failed to create the key directory [%s]", dir)
	}

	scalar := make([]byte, eip712.PrivateKeyLength)
	if _, err := rand.Read(scalar); err != nil {
		return Identity{}, errors.Wrap(err, "evm nwo: failed to generate key material")
	}
	key, err := eip712.DecodePrivateKeyScalar(scalar)
	if err != nil {
		return Identity{}, errors.Wrap(err, "evm nwo: generated an invalid key")
	}

	path := filepath.Join(dir, name+".key")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(scalar)), 0o600); err != nil {
		return Identity{}, errors.Wrapf(err, "evm nwo: failed to write the key for [%s]", name)
	}

	return Identity{Key: key, Address: eip712.PubKeyToAddress(key.PubKey()), Path: path}, nil
}
