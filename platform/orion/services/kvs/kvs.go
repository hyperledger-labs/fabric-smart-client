/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
)

func NewMetadataKVS(kvss *kvs.KVS) driver.MetadataKVS {
	return kvs.NewMetadataKVS[driver.Key, driver.TransientMap](kvss, keyMapper("metadata"))
}

func NewEnvelopeKVS(kvss *kvs.KVS) driver.EnvelopeKVS {
	return kvs.NewEnvelopeKVS[driver.Key](kvss, keyMapper("envelope"))
}

func NewEndorseTxKVS(kvss *kvs.KVS) driver.EndorseTxKVS {
	return kvs.NewEndorseTxKVS[driver.Key](kvss, keyMapper("etx"))
}

func keyMapper(prefix string) kvs.KeyMapper[driver.Key] {
	return func(k driver.Key) (string, error) { return kvs.CreateCompositeKey(prefix, []string{k.Network, k.TxID}) }
}
