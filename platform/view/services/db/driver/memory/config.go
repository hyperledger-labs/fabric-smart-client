/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

var Op = &optsProvider{}

type optsProvider struct{}

func (p *optsProvider) GetOpts(params ...string) sqlite.Opts {
	return sqlite.Opts{
		DataSource:      "file::memory:?cache=shared",
		SkipPragmas:     false,
		MaxOpenConns:    10,
		MaxIdleConns:    common.DefaultMaxIdleConns,
		MaxIdleTime:     common.DefaultMaxIdleTime,
		TablePrefix:     "",
		TableNameParams: params,
	}
}

func (p *optsProvider) GetConfig(params ...string) sqlite.Config {
	return sqlite.Config{
		DataSource:      "file::memory:?cache=shared",
		SkipPragmas:     false,
		MaxOpenConns:    10,
		MaxIdleConns:    common.CopyPtr(common.DefaultMaxIdleConns),
		MaxIdleTime:     common.CopyPtr(common.DefaultMaxIdleTime),
		SkipCreateTable: false,
		TablePrefix:     "",
		TableNameParams: params,
	}
}
