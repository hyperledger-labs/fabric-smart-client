/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
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

var unsupportedStores = collections.NewSet(badger.FilePersistence, badger.BadgerPersistence)

func newBindingStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (driver4.BindingStore, error) {
	driverName := driver4.PersistenceType(utils.DefaultString(in.Config.GetString("fsc.binding.persistence.type"), string(sql.SQLPersistence)))
	if unsupportedStores.Contains(driverName) {
		return binding.NewKVSBased(in.KVS), nil
	}
	for _, d := range in.Drivers {
		if d.Name == driverName {
			return binding.NewWithConfig(d.Driver, "_default", in.Config)
		}
	}
	return nil, errors.New("driver not found")
}

func newSignerInfoStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (driver4.SignerInfoStore, error) {
	driverName := driver4.PersistenceType(utils.DefaultString(in.Config.GetString("fsc.binding.persistence.type"), string(sql.SQLPersistence)))
	if unsupportedStores.Contains(driverName) {
		return signerinfo.NewKVSBased(in.KVS), nil
	}
	for _, d := range in.Drivers {
		if d.Name == driverName {
			return signerinfo.NewWithConfig(d.Driver, "_default", in.Config)
		}
	}
	return nil, errors.New("driver not found")
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
