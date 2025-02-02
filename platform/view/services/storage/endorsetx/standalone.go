/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	"github.com/pkg/errors"
)

const (
	persistenceOptsConfigKey = "fsc.endorsetx.persistence.opts"
	persistenceTypeConfigKey = "fsc.endorsetx.persistence.type"
)

type identifier interface {
	UniqueKey() string
}

func NewWithConfig[K identifier](dbDrivers []driver.NamedDriver, cp db.Config, params ...string) (driver2.EndorseTxStore[K], error) {
	d, err := getDriver(dbDrivers, cp)
	if err != nil {
		return nil, err
	}
	e, err := d.NewEndorseTx(fmt.Sprintf("%s_etx", db.EscapeForTableName(params...)), storage.NewPrefixConfig(cp, persistenceOptsConfigKey))
	if err != nil {
		return nil, err
	}
	return &endorseTxStore[K]{e: e}, nil
}

func getDriver(dbDrivers []driver.NamedDriver, cp db.Config) (driver.Driver, error) {
	var driverName driver2.PersistenceType
	if err := cp.UnmarshalKey(persistenceTypeConfigKey, &driverName); err != nil {
		return nil, err
	}
	for _, d := range dbDrivers {
		if d.Name == driverName {
			return d.Driver, nil
		}
	}
	return &mem.Driver{}, nil
}

type endorseTxStore[K identifier] struct {
	e driver.EndorseTxPersistence
}

func (s *endorseTxStore[K]) GetEndorseTx(key K) ([]byte, error) {
	return s.e.GetEndorseTx(key.UniqueKey())
}
func (s *endorseTxStore[K]) ExistsEndorseTx(key K) (bool, error) {
	return s.e.ExistsEndorseTx(key.UniqueKey())
}
func (s *endorseTxStore[K]) PutEndorseTx(key K, etx []byte) error {
	return s.e.PutEndorseTx(key.UniqueKey(), etx)
}
