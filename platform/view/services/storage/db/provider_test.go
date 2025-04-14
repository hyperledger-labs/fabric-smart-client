/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db_test

import (
	"testing"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var testMatrix = map[string]types.GomegaMatcher{
	"fsc.kvs.persistence": And(
		HaveField("DriverType()", mem.MemoryPersistence),
		HaveField("TableName()", HaveLen(16)),
		HaveField("DbOpts()", And(
			HaveField("Driver()", sql.SQLite),
		)),
	),
}

func TestConfigProvider(t *testing.T) {
	RegisterTestingT(t)

	provider, err := config2.NewProvider("")
	Expect(err).ToNot(HaveOccurred())
	cp := db.NewConfigProvider(provider)
	for configKey, matchCondition := range testMatrix {
		c, err := cp.GetConfig(configKey, "")

		Expect(err).ToNot(HaveOccurred())
		Expect(c).To(matchCondition)
	}
}
