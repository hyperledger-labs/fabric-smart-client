/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

var Op = &optsProvider{}

type optsProvider struct{}

func (p *optsProvider) GetOpts(params ...string) sqlite2.Opts {
	return sqlite2.Opts{
		DataSource:      "file::memory:?cache=shared",
		SkipPragmas:     false,
		MaxOpenConns:    10,
		MaxIdleConns:    common2.DefaultMaxIdleConns,
		MaxIdleTime:     common2.DefaultMaxIdleTime,
		TablePrefix:     "",
		TableNameParams: params,
		Tracing:         nil,
	}
}

func (p *optsProvider) GetConfig(params ...string) sqlite2.Config {
	return sqlite2.Config{
		DataSource:      "file::memory:?cache=shared",
		SkipPragmas:     false,
		MaxOpenConns:    10,
		MaxIdleConns:    common2.CopyPtr(common2.DefaultMaxIdleConns),
		MaxIdleTime:     common2.CopyPtr(common2.DefaultMaxIdleTime),
		SkipCreateTable: false,
		TablePrefix:     "",
		TableNameParams: params,
	}
}
