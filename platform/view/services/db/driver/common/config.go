/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	DefaultMaxIdleConns                         = 2
	DefaultMaxIdleTime                          = time.Minute
	DefaultPersistence  driver2.PersistenceName = "default"
)

type configService interface {
	// UnmarshalKey takes the value corresponding to the passed key and unmarshals it into the passed structure
	UnmarshalKey(key string, rawVal interface{}) error
}

func NewConfig(cs configService) *config {
	return &config{cs: cs}
}

func GetPersistenceName(cs configService, prefix string) driver2.PersistenceName {
	var persistence driver2.PersistenceName
	utils.Must(cs.UnmarshalKey(prefix, &persistence))
	return persistence
}

type config struct {
	cs configService
}

func (c *config) GetDriverType(name driver2.PersistenceName) (driver.PersistenceType, error) {
	var d driver.PersistenceType
	err := c.unmarshalPersistenceKey(name, "type", &d)
	return d, err
}

func (c *config) UnmarshalDriverOpts(name driver2.PersistenceName, v any) error {
	return c.unmarshalPersistenceKey(name, "opts", v)
}

func (c *config) unmarshalPersistenceKey(name driver2.PersistenceName, key string, v any) error {
	if len(name) == 0 {
		name = DefaultPersistence
	}
	return c.cs.UnmarshalKey(fmt.Sprintf("fsc.persistences.%s.%s", name, key), v)
}
