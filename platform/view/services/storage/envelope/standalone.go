/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	"github.com/pkg/errors"
)

const (
	persistenceOptsConfigKey = "fsc.envelope.persistence.opts"
	persistenceTypeConfigKey = "fsc.envelope.persistence.type"
)

type identifier interface {
	UniqueKey() string
}

func NewWithConfig[K identifier](dbDrivers []driver.NamedDriver, cp db.Config, params ...string) (driver2.EnvelopeStore[K], error) {
	d, err := getDriver(dbDrivers, cp)
	if err != nil {
		return nil, err
	}
	e, err := d.NewEnvelope(fmt.Sprintf("%s_env", db.EscapeForTableName(params...)), storage.NewPrefixConfig(cp, persistenceOptsConfigKey))
	if err != nil {
		return nil, err
	}
	return &envelopeStore[K]{e: e}, nil
}

var supportedStores = collections.NewSet(mem.MemoryPersistence, sql.SQLPersistence)

func getDriver(dbDrivers []driver.NamedDriver, cp db.Config) (driver.Driver, error) {
	var driverName driver2.PersistenceType
	if err := cp.UnmarshalKey(persistenceTypeConfigKey, &driverName); err != nil {
		return nil, err
	}
	if !supportedStores.Contains(driverName) {
		driverName = mem.MemoryPersistence
	}
	for _, d := range dbDrivers {
		if d.Name == driverName {
			return d.Driver, nil
		}
	}
	return nil, errors.New("driver not found")
}

type envelopeStore[K identifier] struct {
	e driver.EnvelopePersistence
}

func (s *envelopeStore[K]) GetEnvelope(key K) ([]byte, error) {
	return s.e.GetEnvelope(key.UniqueKey())
}
func (s *envelopeStore[K]) ExistsEnvelope(key K) (bool, error) {
	return s.e.ExistsEnvelope(key.UniqueKey())
}
func (s *envelopeStore[K]) PutEnvelope(key K, etx []byte) error {
	return s.e.PutEnvelope(key.UniqueKey(), etx)
}
