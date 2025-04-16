/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
)

func NewEndorseTxStore(config driver2.ConfigService, drivers common2.Driver) (driver.EndorseTxStore, error) {
	return endorsetx.NewEndorseTx[driver.Key](config, drivers, "default")
}

func NewMetadataStore(config driver2.ConfigService, drivers common2.Driver) (driver.MetadataStore, error) {
	return metadata.NewStore[driver.Key, driver.TransientMap](config, drivers, "default")
}

func NewEnvelopeStore(config driver2.ConfigService, drivers common2.Driver) (driver.EnvelopeStore, error) {
	return envelope.NewEnvelope[driver.Key](config, drivers, "default")
}
