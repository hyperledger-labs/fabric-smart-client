/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
)

func NewEndorseTxStore(c *storage.Constructor) (driver.EndorseTxStore, error) {
	e, err := c.NewEndorseTx("default")
	if err != nil {
		return nil, err
	}
	return endorsetx.NewEndorseTxStore[driver.Key](e), nil
}

func NewMetadataStore(c *storage.Constructor) (driver.MetadataStore, error) {
	m, err := c.NewMetadata("default")
	if err != nil {
		return nil, err
	}
	return metadata.NewMetadataStore[driver.Key, driver.TransientMap](m), nil
}

func NewEnvelopeStore(c *storage.Constructor) (driver.EnvelopeStore, error) {
	e, err := c.NewEnvelope("default")
	if err != nil {
		return nil, err
	}
	return envelope.NewEnvelopeStore[driver.Key](e), nil
}
