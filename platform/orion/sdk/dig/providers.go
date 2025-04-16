/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
	"go.uber.org/dig"
)

func newEndorseTxStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.EndorseTxStore, error) {
	return endorsetx.NewStore[driver.Key](in.Config, in.Drivers, "default")
}

func newMetadataStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.MetadataStore, error) {
	return metadata.NewStore[driver.Key, driver.TransientMap](in.Config, in.Drivers, "default")
}

func newEnvelopeStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.EnvelopeStore, error) {
	return envelope.NewStore[driver.Key](in.Config, in.Drivers, "default")
}
