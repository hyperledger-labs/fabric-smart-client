/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	kvs2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
	"github.com/pkg/errors"
	"go.uber.org/dig"
)

func newKVS(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (*kvs.KVS, error) {
	size, err := kvs.CacheSizeFromConfig(in.Config)
	if err != nil {
		return nil, err
	}
	kvss, err := kvs2.NewStore(in.Config, in.Drivers)
	if err != nil {
		return nil, err
	}
	return kvs.New(utils.MustGet(kvss, err), "_default", size)
}

func newBindingStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.BindingStore, error) {
	return binding.NewStore(in.Config, in.Drivers, "default")
}

func newSignerInfoStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.SignerInfoStore, error) {
	return signerinfo.NewStore(in.Config, in.Drivers, "default")
}

func newAuditInfoStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.AuditInfoStore, error) {
	return auditinfo.NewStore(in.Config, in.Drivers, "default")
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
