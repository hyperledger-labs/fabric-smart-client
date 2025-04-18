/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
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
	db, err := sql.Open(driverName, opts.DataSource)
	if err != nil {
		logger.Error(err)
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxIdleTime(opts.MaxIdleTime)

	if err = db.Ping(); err != nil {
		return nil, err
	}
	logger.Debugf("connected to [%s] for reads, max open connections: %d, max idle connections: %d, max idle time: %v", driverName, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)

	return &common.RWDB{
		ReadDB:  db,
		WriteDB: db,
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
