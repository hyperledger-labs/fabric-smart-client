/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
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

func GetTableNames(prefix string, params ...string) TableNames {
	nc := utils.MustGet(ncProvider.GetFormatter(prefix))
	return TableNames{
		EndorseTx: nc.MustFormat("etx", params...),
		Metadata:  nc.MustFormat("meta", params...),
		Envelope:  nc.MustFormat("env", params...),
		State:     nc.MustFormat("vstate", params...),
		Status:    nc.MustFormat("vstatus", params...),
	}
}
