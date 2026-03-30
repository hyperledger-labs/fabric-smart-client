/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
)

var logger = logging.MustGetLogger()

// New creates a new Vault instance for the specified channel using the provided configuration
// and query service provider.
//
// It retrieves the query service for the network and channel from the provider, then creates
// a Vault that uses this query service for remote state queries.
//
// Parameters:
//   - configService: Configuration service providing network name and other settings
//   - channel: The channel name for which to create the vault
//   - queryServiceProvider: Provider for obtaining the query service instance
//
// Returns:
//   - *Vault: A new vault instance configured with the query service
//   - error: An error if the query service cannot be obtained
func New(configService fdriver.ConfigService, channel string, queryServiceProvider queryservice.Provider) (*Vault, error) {
	queryService, err := queryServiceProvider.Get(configService.NetworkName(), channel)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting query service")
	}
	return NewVault(queryService), nil
}
