/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
)

var ncProvider = db.NewTableNameCreator("fsc")

type PersistenceConstructor[V common.DBObject] func(*common.RWDB, TableNames) (V, error)

type TracingConfig struct{}

type TableNames struct {
	KVS        string
	Binding    string
	SignerInfo string
	AuditInfo  string
}

// GetTableNames computes the KVS, binding, signer-info and audit-info table names for prefix,
// using params to keep names distinct across callers sharing the same prefix (e.g. per-channel
// stores). It returns an error if prefix or params can't be turned into valid table name identifiers.
func GetTableNames(prefix string, params ...string) (TableNames, error) {
	nc, err := ncProvider.GetFormatter(prefix)
	if err != nil {
		return TableNames{}, errors.Wrapf(err, "failed to get table name formatter for prefix [%s]", prefix)
	}
	kvs, err := nc.Format("kvs", params...)
	if err != nil {
		return TableNames{}, err
	}
	binding, err := nc.Format("bind", params...)
	if err != nil {
		return TableNames{}, err
	}
	signerInfo, err := nc.Format("sign", params...)
	if err != nil {
		return TableNames{}, err
	}
	auditInfo, err := nc.Format("aud", params...)
	if err != nil {
		return TableNames{}, err
	}
	return TableNames{
		KVS:        kvs,
		Binding:    binding,
		SignerInfo: signerInfo,
		AuditInfo:  auditInfo,
	}, nil
}
