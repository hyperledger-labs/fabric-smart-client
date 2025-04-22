/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
)

func newEndorseTxStore(config vdriver.ConfigService, drivers multiplexed.Driver) (driver.EndorseTxStore, error) {
	return endorsetx.NewStore[driver.Key](config, drivers, "default")
}

func newMetadataStore(config vdriver.ConfigService, drivers multiplexed.Driver) (driver.MetadataStore, error) {
	return metadata.NewStore[driver.Key, driver.TransientMap](config, drivers, "default")
}

func newEnvelopeStore(config vdriver.ConfigService, drivers multiplexed.Driver) (driver.EnvelopeStore, error) {
	return envelope.NewStore[driver.Key](config, drivers, "default")
}
