/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	kvs2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
	"github.com/pkg/errors"
	"go.uber.org/dig"
)

func newKVS(driver multiplexed.Driver, config driver.ConfigService) (*kvs.KVS, error) {
	size, err := kvs.CacheSizeFromConfig(config)
	if err != nil {
		return nil, err
	}
	kvss, err := kvs2.NewStore(config, driver)
	if err != nil {
		return nil, err
	}
	return kvs.New(utils.MustGet(kvss, err), "_default", size)
}

func newBinding(driver multiplexed.Driver, config driver.ConfigService) (driver4.BindingStore, error) {
	return binding.NewStore(config, driver, "default")
}

func newSignerInfo(driver multiplexed.Driver, config driver.ConfigService) (driver4.SignerInfoStore, error) {
	return signerinfo.NewStore(config, driver, "default")
}

func newAuditInfo(driver multiplexed.Driver, config driver.ConfigService) (driver4.AuditInfoStore, error) {
	return auditinfo.NewStore(config, driver, "default")
}

func newCommonDbDriver(in struct {
	dig.In
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) multiplexed.Driver {
	return in.Drivers
}

func newKMSDriver(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []driver3.NamedDriver `group:"kms-drivers"`
}) (*kms.KMS, error) {
	driverName := utils.DefaultString(in.Config.GetString("fsc.identity.type"), "file")
	for _, driver := range in.Drivers {
		if string(driver.Name) == driverName {
			return &kms.KMS{Driver: driver.Driver}, nil
		}
	}
	return nil, errors.New("driver not found")
}
