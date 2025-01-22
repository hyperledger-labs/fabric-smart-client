/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/pkg/errors"
	"go.uber.org/dig"
)

func NewEndorseTxStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.EndorseTxStore, error) {
	driverName := driver2.PersistenceType(utils.DefaultString(in.Config.GetString("fsc.endorsetx.persistence.type"), string(mem.MemoryPersistence)))
	if sdk.UnsupportedStores.Contains(driverName) {
		return services.NewKVSBasedEndorseTxStore(in.KVS), nil
	}
	for _, d := range in.Drivers {
		if d.Name == driverName {
			return services.NewDBBasedEndorseTxStore(d.Driver, "_default", in.Config)
		}
	}
	return nil, errors.New("driver not found")
}

func NewMetadataStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.MetadataStore, error) {
	driverName := driver2.PersistenceType(utils.DefaultString(in.Config.GetString("fsc.metadata.persistence.type"), string(mem.MemoryPersistence)))
	if sdk.UnsupportedStores.Contains(driverName) {
		return services.NewKVSBasedMetadataStore(in.KVS), nil
	}
	for _, d := range in.Drivers {
		if d.Name == driverName {
			return services.NewDBBasedMetadataStore(d.Driver, "_default", in.Config)
		}
	}
	return nil, errors.New("driver not found")
}
