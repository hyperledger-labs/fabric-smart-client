/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var logger = flogging.MustGetLogger("orion-sdk.db")

type Opts struct {
	Network  string
	Database string
	Creator  string
}

type Driver struct {
	lock sync.Mutex
	ons  map[string]OrionBackend
}

func (o *Driver) NewVersioned(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	opts := &Opts{}
	err := config.UnmarshalKey("", opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting opts")
	}
	logger.Debugf("opening orion db for [%s:%s:%s]", opts.Network, opts.Database, opts.Creator)
	return o.OpenDB(sp, opts.Network, opts.Database, opts.Creator)
}

func (o *Driver) New(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.Persistence, error) {
	db, err := o.NewVersioned(sp, dataSourceName, config)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create orion driver for [%s]", dataSourceName)
	}
	return &unversioned.Unversioned{Versioned: db}, nil
}

func (o *Driver) OpenDB(sp view.ServiceProvider, onsName, dbName, creator string) (*Orion, error) {
	o.lock.Lock()
	defer o.lock.Unlock()

	backend, ok := o.ons[onsName]
	if !ok {
		c, err := config.New(view.GetConfigService(sp), onsName, false)
		if err != nil {
			return nil, err
		}
		ons, err := generic.NewDB(context.Background(), sp, c, onsName)
		if err != nil {
			return nil, err
		}
		backend = orion.NewNetworkService(sp, ons, onsName)
		o.ons[onsName] = backend
	}

	return &Orion{
		name:      dbName,
		ons:       backend,
		txManager: backend.TransactionManager(),
		creator:   creator,
	}, nil
}

func init() {
	db.Register("orion", &Driver{
		ons: make(map[string]OrionBackend),
	})
}
