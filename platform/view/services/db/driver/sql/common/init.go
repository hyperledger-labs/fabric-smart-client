/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
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

func GetTableNames(prefix string, params ...string) TableNames {
	nc := utils.MustGet(ncProvider.GetFormatter(prefix))
	return TableNames{
		KVS:        nc.MustFormat("kvs", params...),
		Binding:    nc.MustFormat("bind", params...),
		SignerInfo: nc.MustFormat("sign", params...),
		AuditInfo:  nc.MustFormat("aud", params...),
	}
}
