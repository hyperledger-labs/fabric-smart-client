/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import (
	"os"
	"strconv"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// External describes an EVM endpoint the platform uses instead of starting Besu.
type External struct {
	Endpoint  string
	ChainID   int64
	FunderKey string
}

// ExternalFromEnv reads EVM_ENDPOINT, EVM_CHAIN_ID and EVM_FUNDER_KEY. It returns ok == false when
// none of the three are set, which selects Besu mode; the parsed values when all three are set;
// and an error naming whichever variables are missing otherwise.
func ExternalFromEnv() (*External, bool, error) {
	endpoint := os.Getenv("EVM_ENDPOINT")
	chainIDStr := os.Getenv("EVM_CHAIN_ID")
	funderKey := os.Getenv("EVM_FUNDER_KEY")

	if endpoint == "" && chainIDStr == "" && funderKey == "" {
		return nil, false, nil
	}

	var missing []string
	if endpoint == "" {
		missing = append(missing, "EVM_ENDPOINT")
	}
	if chainIDStr == "" {
		missing = append(missing, "EVM_CHAIN_ID")
	}
	if funderKey == "" {
		missing = append(missing, "EVM_FUNDER_KEY")
	}
	if len(missing) > 0 {
		return nil, false, errors.Errorf(
			"evm nwo: set all of EVM_ENDPOINT, EVM_CHAIN_ID and EVM_FUNDER_KEY, or none; missing %s",
			strings.Join(missing, ", "))
	}

	chainID, err := strconv.ParseInt(chainIDStr, 10, 64)
	if err != nil {
		return nil, false, errors.Wrapf(err, "evm nwo: invalid EVM_CHAIN_ID [%s]", chainIDStr)
	}

	return &External{Endpoint: endpoint, ChainID: chainID, FunderKey: funderKey}, true, nil
}
