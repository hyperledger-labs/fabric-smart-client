/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
)

func NewEndorseTxStore(config dbdriver.Config, drivers common2.Driver) (driver.EndorseTxStore, error) {
	return endorsetx.NewEndorseTx[driver.Key](config, drivers, "default")
}

func NewMetadataStore(config dbdriver.Config, drivers common2.Driver) (driver.MetadataStore, error) {
	return metadata.NewStore[driver.Key, driver.TransientMap](config, drivers, "default")
}

func NewEnvelopeStore(config dbdriver.Config, drivers common2.Driver) (driver.EnvelopeStore, error) {
	return envelope.NewEnvelope[driver.Key](config, drivers, "default")
}
