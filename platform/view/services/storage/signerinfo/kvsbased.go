/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewKVSBased(kvss *kvs.KVS) *signerKVS {
	return &signerKVS{e: kvs.NewEnhancedKVS[view.Identity, *sig.SignerEntry](kvss, signerKey)}
}

func signerKey(id view.Identity) (string, error) {
	return kvs.CreateCompositeKey("sigService", []string{"signer", id.UniqueID()})
}

type signerKVS struct {
	e *kvs.EnhancedKVS[view.Identity, *sig.SignerEntry]
}

func (kvs *signerKVS) GetSigner(id view.Identity) (*sig.SignerEntry, error) { return kvs.e.Get(id) }
func (kvs *signerKVS) FilterExistingSigners(ids ...view.Identity) ([]view.Identity, error) {
	return kvs.e.FilterExisting(ids...)
}
func (kvs *signerKVS) PutSigner(id view.Identity, entry *sig.SignerEntry) error {
	return kvs.e.Put(id, entry)
}
