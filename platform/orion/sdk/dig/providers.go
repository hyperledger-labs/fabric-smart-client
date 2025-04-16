/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
)

func newEndorseTxStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver.EndorseTxStore, error) {
	return endorsetx.NewEndorseTx[driver.Key](config, drivers, "default")
}

func newMetadataStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver.MetadataStore, error) {
	return metadata.NewStore[driver.Key, driver.TransientMap](config, drivers, "default")
}

func newEnvelopeStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver.EnvelopeStore, error) {
	return envelope.NewEnvelope[driver.Key](config, drivers, "default")
}

func newBindingStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver3.BindingStore, error) {
	return binding.NewStore(config, drivers, "default")
}

func newSignerInfoStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver3.SignerInfoStore, error) {
	return signerinfo.NewStore(config, drivers, "default")
}

func newAuditInfoStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver3.AuditInfoStore, error) {
	return auditinfo.NewStore(config, drivers, "default")
}
