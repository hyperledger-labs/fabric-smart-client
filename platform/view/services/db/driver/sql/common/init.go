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
		KVS:        nc.MustCreateTableName(prefix, "kvs", params...),
		Binding:    nc.MustCreateTableName(prefix, "bind", params...),
		SignerInfo: nc.MustCreateTableName(prefix, "sign", params...),
		AuditInfo:  nc.MustCreateTableName(prefix, "aud", params...),
		EndorseTx:  nc.MustCreateTableName(prefix, "etx", params...),
		Metadata:   nc.MustCreateTableName(prefix, "meta", params...),
		Envelope:   nc.MustCreateTableName(prefix, "env", params...),
		State:      nc.MustCreateTableName(prefix, "vstate", params...),
		Status:     nc.MustCreateTableName(prefix, "vstatus", params...),
	}
}
