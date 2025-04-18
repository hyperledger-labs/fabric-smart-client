/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
)

func MockTypeConfig[T any](typ driver.PersistenceType, config T) *mock.ConfigProvider {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(key string, val interface{}) error {
		if strings.Contains(key, "type") {
			if typ == mem.Persistence {
				*val.(*driver.PersistenceType) = mem.Persistence
			} else if typ == postgres.Persistence || typ == sqlite.Persistence {
				*val.(*driver.PersistenceType) = sql.SQLPersistence
			} else {
				panic("not defined type " + typ)
			}
		} else if strings.Contains(key, "opts.driver") {
			if typ == postgres.Persistence {
				*val.(*driver2.SQLDriverType) = sql.Postgres
			} else if typ == sqlite.Persistence {
				*val.(*driver2.SQLDriverType) = sql.SQLite
			} else {
				panic("not defined type " + typ)
			}
		} else if strings.Contains(key, "opts") {
			*val.(*T) = config
		}
		return nil
	})
	return cp
}
