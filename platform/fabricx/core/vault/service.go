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

func New(configService fdriver.ConfigService, channel string, queryServiceProvider queryservice.Provider) (*VaultX, error) {
	queryService, err := queryServiceProvider.Get(configService.NetworkName(), channel)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting query service")
	}
	return NewVaultX(queryService), nil
}
