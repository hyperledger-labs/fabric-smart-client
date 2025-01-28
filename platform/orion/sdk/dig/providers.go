/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"go.uber.org/dig"
)

func NewEndorseTxStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.EndorseTxStore, error) {
	if store, err := services.NewDBBasedEndorseTxStore(in.Drivers, in.Config, "default"); err != nil {
		logger.Errorf("failed creating store for etx: %v. Default to KVS", err)
		return services.NewKVSBasedEndorseTxStore(in.KVS), nil
	} else {
		return store, nil
	}
}

func NewMetadataStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.MetadataStore, error) {
	if store, err := services.NewDBBasedMetadataStore(in.Drivers, in.Config, "default"); err != nil {
		logger.Errorf("failed creating store for meta: %v. Default to KVS", err)
		return services.NewKVSBasedMetadataStore(in.KVS), nil
	} else {
		return store, nil
	}
}

func NewEnvelopeStore(in struct {
	dig.In
	KVS     *kvs.KVS
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.EnvelopeStore, error) {
	if store, err := services.NewDBBasedEnvelopeStore(in.Drivers, in.Config, "default"); err != nil {
		logger.Errorf("failed creating store for env: %v. Default to KVS", err)
		return services.NewKVSBasedEnvelopeStore(in.KVS), nil
	} else {
		return store, nil
	}
}
