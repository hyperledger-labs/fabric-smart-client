/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auditinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewKVSBased(kvss *kvs.KVS) *auditInfoKVS {
	return &auditInfoKVS{e: kvs.NewEnhancedKVS[view.Identity, []byte](kvss, auditInfoKey)}
}

func auditInfoKey(id view.Identity) (string, error) {
	return kvs.CreateCompositeKey("fsc.platform.view.sig", []string{id.UniqueID()})
}

type auditInfoKVS struct {
	e *kvs.EnhancedKVS[view.Identity, []byte]
}

func (kvs *auditInfoKVS) GetAuditInfo(id view.Identity) ([]byte, error) { return kvs.e.Get(id) }
func (kvs *auditInfoKVS) PutAuditInfo(id view.Identity, info []byte) error {
	return kvs.e.Put(id, info)
}
