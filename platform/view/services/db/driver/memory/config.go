/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

var op = &optsProvider{}

var memOpts = sqlite.Opts{
	DataSource:   "file::memory:?cache=shared",
	SkipPragmas:  false,
	MaxOpenConns: 10,
	MaxIdleConns: common.DefaultMaxIdleConns,
	MaxIdleTime:  common.DefaultMaxIdleTime,
	TablePrefix:  utils.GenerateUUIDOnlyLetters(),
}

type optsProvider struct{}

func (p *optsProvider) GetOpts() sqlite.Opts { return memOpts }
