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
	EndorseTx string
	Metadata  string
	Envelope  string
	State     string
	Status    string
}

// GetTableNames computes the endorse-tx, metadata, envelope, vault-state and vault-status table
// names for prefix, using params to keep names distinct across callers sharing the same prefix
// (e.g. per-channel stores). It returns an error if prefix or params can't be turned into valid
// table name identifiers.
func GetTableNames(prefix string, params ...string) (TableNames, error) {
	nc, err := ncProvider.GetFormatter(prefix)
	if err != nil {
		return TableNames{}, errors.Wrapf(err, "failed to get table name formatter for prefix [%s]", prefix)
	}
	endorseTx, err := nc.Format("etx", params...)
	if err != nil {
		return TableNames{}, err
	}
	metadata, err := nc.Format("meta", params...)
	if err != nil {
		return TableNames{}, err
	}
	envelope, err := nc.Format("env", params...)
	if err != nil {
		return TableNames{}, err
	}
	state, err := nc.Format("vstate", params...)
	if err != nil {
		return TableNames{}, err
	}
	status, err := nc.Format("vstatus", params...)
	if err != nil {
		return TableNames{}, err
	}
	return TableNames{
		EndorseTx: endorseTx,
		Metadata:  metadata,
		Envelope:  envelope,
		State:     state,
		Status:    status,
	}, nil
}
