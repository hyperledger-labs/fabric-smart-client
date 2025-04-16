/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/pkg/errors"
)

type Driver []driver2.NamedDriver

func (d Driver) NewKVS(name string, cfg driver2.Config) (driver2.UnversionedPersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewKVS(name, cfg)
}

func (d Driver) NewBinding(name string, cfg driver2.Config) (driver2.BindingPersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewBinding(name, cfg)
}

func (d Driver) NewSignerInfo(name string, cfg driver2.Config) (driver2.SignerInfoPersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewSignerInfo(name, cfg)
}

func (d Driver) NewAuditInfo(name string, cfg driver2.Config) (driver2.AuditInfoPersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewAuditInfo(name, cfg)
}

func (d Driver) NewEndorseTx(name string, cfg driver2.Config) (driver2.EndorseTxPersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewEndorseTx(name, cfg)
}

func (d Driver) NewMetadata(name string, cfg driver2.Config) (driver2.MetadataPersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewMetadata(name, cfg)
}

func (d Driver) NewEnvelope(name string, cfg driver2.Config) (driver2.EnvelopePersistence, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewEnvelope(name, cfg)
}

func (d Driver) NewVault(name string, cfg driver2.Config) (driver.VaultStore, error) {
	dr, err := d.getDriver(cfg)
	if err != nil {
		return nil, err
	}
	return dr.NewVault(name, cfg)
}

func (d Driver) getDriver(c driver2.Config) (driver2.Driver, error) {
	t, err := getDriverType(c)
	if err != nil {
		return nil, err
	}
	for _, dr := range d {
		if dr.Name == t {
			return dr.Driver, nil
		}
	}
	return nil, errors.Errorf("driver %s not found", t)
}

func getDriverType(c driver2.Config) (driver.PersistenceType, error) {
	var d driver.PersistenceType
	if err := c.UnmarshalKey("type", &d); err != nil {
		return "", err
	}
	if len(d) == 0 || d == mem.Persistence {
		return mem.Persistence, nil
	}
	if d != sql.SQLPersistence {
		return "", errors.Errorf("unknown persistence type: [%s]", d)
	}
	var t driver2.SQLDriverType
	if err := c.UnmarshalKey("opts.driver", &t); err != nil {
		return "", err
	}
	if t == sql.SQLite {
		return sqlite.Persistence, nil
	}
	if t == sql.Postgres {
		return postgres.Persistence, nil
	}
	return "", errors.Errorf("type [%s] not defined", t)
}
