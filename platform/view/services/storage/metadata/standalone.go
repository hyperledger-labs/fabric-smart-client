/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"encoding/json"
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
	persistenceOptsConfigKey = "fsc.metadata.persistence.opts"
	persistenceTypeConfigKey = "fsc.metadata.persistence.type"
)

type identifier interface {
	UniqueKey() string
}

func NewWithConfig[K identifier, M any](dbDrivers []driver.NamedDriver, cp db.Config, params ...string) (driver2.MetadataStore[K, M], error) {
	d, err := getDriver(dbDrivers, cp)
	if err != nil {
		return nil, err
	}
	m, err := d.NewMetadata(fmt.Sprintf("%s_meta", db.EscapeForTableName(params...)), storage.NewPrefixConfig(cp, persistenceOptsConfigKey))
	if err != nil {
		return nil, err
	}
	return &metadataStore[K, M]{m: m}, nil
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

type metadataStore[K identifier, M any] struct {
	m driver.MetadataPersistence
}

func (s *metadataStore[K, M]) GetMetadata(key K) (M, error) {
	var m M
	data, err := s.m.GetMetadata(key.UniqueKey())
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}
func (s *metadataStore[K, M]) ExistMetadata(key K) (bool, error) {
	return s.m.ExistMetadata(key.UniqueKey())
}
func (s *metadataStore[K, M]) PutMetadata(key K, m M) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.m.PutMetadata(key.UniqueKey(), data)
}
