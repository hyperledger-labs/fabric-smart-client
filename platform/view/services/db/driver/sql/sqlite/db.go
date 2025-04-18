/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
)

var DbProvider = newDBProvider()

type dbProvider struct {
	p lazy.Provider[Opts, *common.RWDB]
}

func newDBProvider() *dbProvider {
	return &dbProvider{p: lazy.NewProviderWithKeyMapper(key, open)}
}

func open(opts Opts) (*common.RWDB, error) {
	logger.Debugf("Opening read db [%v]", opts.DataSource)
	readDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("can't open read %s database: %w", driverName, err)
	}
	logger.Debugf("Opening write db [%v]", opts.DataSource)
	writeDB, err := OpenDB(opts.DataSource, 1, maxIdleConnsWrite, maxIdleTimeWrite, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("can't open write %s database: %w", driverName, err)
	}
	return &common.RWDB{
		ReadDB:  readDB,
		WriteDB: writeDB,
	}, nil
}

func (p *dbProvider) OpenDB(opts Opts) (*common.RWDB, error) {
	return open(opts)
}

func (p *dbProvider) GetOrOpen(opts Opts) (*common.RWDB, error) {
	if _, ok := p.p.Peek(opts); ok {
		logger.Infof("DB [%s] already exists. Returning cached DB", opts.DataSource)
	} else {
		logger.Infof("Creating new DB for [%s]", opts.DataSource)
	}
	return p.p.Get(opts)
}

func key(o Opts) string {
	return o.DataSource
}
