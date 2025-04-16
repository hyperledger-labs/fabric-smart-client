/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"

var nc = db.NewTableNameCreator()

type TableNames struct {
	KVS        string
	Binding    string
	SignerInfo string
	AuditInfo  string
	EndorseTx  string
	Metadata   string
	Envelope   string
	State      string
	Status     string
}

func GetTableNames(prefix string, params ...string) TableNames {
	return TableNames{
		KVS:        nc.MustGetTableName(prefix, "kvs", params...),
		Binding:    nc.MustGetTableName(prefix, "bind", params...),
		SignerInfo: nc.MustGetTableName(prefix, "sign", params...),
		AuditInfo:  nc.MustGetTableName(prefix, "aud", params...),
		EndorseTx:  nc.MustGetTableName(prefix, "etx", params...),
		Metadata:   nc.MustGetTableName(prefix, "meta", params...),
		Envelope:   nc.MustGetTableName(prefix, "env", params...),
		State:      nc.MustGetTableName(prefix, "vstate", params...),
		Status:     nc.MustGetTableName(prefix, "vstatus", params...),
	}
}
