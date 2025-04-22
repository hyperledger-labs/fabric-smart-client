/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"

const (
	KvsPersistencePrefix        = "fsc_kvs"
	AuditInfoPersistencePrefix  = "fsc_auditinfo"
	SignerInfoPersistencePrefix = "fsc_signerinfo"
	BindingPersistencePrefix    = "fsc_binding"
	EndorseTxPersistencePrefix  = "fsc_endorsetx"
	MetadataPersistencePrefix   = "fsc_metadata"
	EnvelopePersistencePrefix   = "fsc_envelope"
)

var AllPrefixes = []node.PersistenceKey{
	KvsPersistencePrefix,
	BindingPersistencePrefix,
	AuditInfoPersistencePrefix,
	SignerInfoPersistencePrefix,
	EndorseTxPersistencePrefix,
	EnvelopePersistencePrefix,
	MetadataPersistencePrefix,
}
