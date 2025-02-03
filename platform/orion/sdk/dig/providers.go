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
	"go.uber.org/dig"
)

func NewEndorseTxStore(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.EndorseTxStore, error) {
	return services.NewDBBasedEndorseTxStore(in.Drivers, in.Config, "default")
}

func NewMetadataStore(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.MetadataStore, error) {
	return services.NewDBBasedMetadataStore(in.Drivers, in.Config, "default")
}

func NewEnvelopeStore(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver3.EnvelopeStore, error) {
	return services.NewDBBasedEnvelopeStore(in.Drivers, in.Config, "default")
}
