/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/metadata"
)

func NewKVSBasedMetadataStore(kvss *kvs.KVS) driver.MetadataStore {
	return metadata.NewKVSBased[driver.Key, driver.TransientMap](kvss, keyMapper("metadata"))
}

func NewKVSBasedEnvelopeStore(kvss *kvs.KVS) driver.EnvelopeStore {
	return envelope.NewKVSBased[driver.Key](kvss, keyMapper("envelope"))
}

func NewKVSBasedEndorseTxStore(kvss *kvs.KVS) driver.EndorseTxStore {
	return endorsetx.NewKVSBased[driver.Key](kvss, keyMapper("etx"))
}

func keyMapper(prefix string) kvs.KeyMapper[driver.Key] {
	return func(k driver.Key) (string, error) { return kvs.CreateCompositeKey(prefix, []string{k.Network, k.TxID}) }
}

func NewDBBasedEndorseTxStore(dbDrivers []driver2.NamedDriver, cp db.Config, namespace string) (driver.EndorseTxStore, error) {
	return endorsetx.NewWithConfig[driver.Key](dbDrivers, cp, namespace)
}

func NewDBBasedMetadataStore(dbDrivers []driver2.NamedDriver, cp db.Config, namespace string) (driver.MetadataStore, error) {
	return metadata.NewWithConfig[driver.Key, driver.TransientMap](dbDrivers, cp, namespace)
}

func NewDBBasedEnvelopeStore(dbDrivers []driver2.NamedDriver, cp db.Config, namespace string) (driver.EnvelopeStore, error) {
	return envelope.NewWithConfig[driver.Key](dbDrivers, cp, namespace)
}
