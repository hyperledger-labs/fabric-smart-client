/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	kvs2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/pkg/errors"
	"go.uber.org/dig"
)

func newMultiplexedDriver(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) multiplexed.Driver {
	return multiplexed.NewDriver(in.Config, in.Drivers...)
}

func newKVS(config vdriver.ConfigService, driver multiplexed.Driver) (*kvs.KVS, error) {
	size, err := kvs.CacheSizeFromConfig(config)
	if err != nil {
		return nil, err
	}

	return kvs.New(utils.MustGet(kvs2.NewStore(config, driver)), "_default", size)
}

func newKMSDriver(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []driver2.NamedDriver `group:"kms-drivers"`
}) (*kms.KMS, error) {
	driverName := utils.DefaultString(in.Config.GetString("fsc.identity.type"), "file")
	for _, driver := range in.Drivers {
		if string(driver.Name) == driverName {
			return &kms.KMS{Driver: driver.Driver}, nil
		}
	}
	return nil, errors.New("driver not found")
}
