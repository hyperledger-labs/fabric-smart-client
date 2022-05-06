/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("orion-sdk.db")

type Opts struct {
	Network  string
	Database string
	Creator  string
}

type Driver struct {
}

func (o *Driver) NewVersioned(sp view2.ServiceProvider, dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	opts := &Opts{}
	err := config.UnmarshalKey("", opts)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting opts")
	}
	logger.Debugf("opening orion db for [%s:%s:%s]", opts.Network, opts.Database, opts.Creator)
	return OpenDB(sp, opts.Network, opts.Database, opts.Creator)
}

func (o *Driver) New(sp view2.ServiceProvider, dataSourceName string, config driver.Config) (driver.Persistence, error) {
	db, err := o.NewVersioned(sp, dataSourceName, config)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create orion driver for [%s]", dataSourceName)
	}
	return &unversioned.Unversioned{Versioned: db}, nil
}

func init() {
	db.Register("orion", &Driver{})
}
