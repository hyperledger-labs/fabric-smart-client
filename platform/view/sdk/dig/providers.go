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
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
	"github.com/pkg/errors"
	"go.uber.org/dig"
)

func newKVS(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (*kvs.KVS, error) {
	driverName := utils.DefaultString(in.Config.GetString("fsc.kvs.persistence.type"), string(mem.MemoryPersistence))
	for _, driver := range in.Drivers {
		if string(driver.Name) == driverName {
			return kvs.NewWithConfig(driver.Driver, "_default", in.Config)
		}
	}
	return nil, errors.New("driver not found")
}

func newBindingStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (driver4.BindingStore, error) {
	return binding.NewWithConfig(in.Drivers, in.Config, "default")
}

func newSignerInfoStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (driver4.SignerInfoStore, error) {
	return signerinfo.NewWithConfig(in.Drivers, in.Config, "default")
}

func newAuditInfoStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (driver4.AuditInfoStore, error) {
	return auditinfo.NewWithConfig(in.Drivers, in.Config, "default")
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
