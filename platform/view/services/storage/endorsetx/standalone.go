/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	persistenceOptsConfigKey = "fsc.endorsetx.persistence.opts"
)

type identifier interface {
	UniqueKey() string
}

func NewWithConfig[K identifier](dbDriver driver.Driver, namespace string, cp db.Config) (driver2.EndorseTxStore[K], error) {
	e, err := dbDriver.NewEndorseTx(fmt.Sprintf("%s_etx", namespace), db.NewPrefixConfig(cp, persistenceOptsConfigKey))
	if err != nil {
		return nil, err
	}
	return &endorseTxStore[K]{e: e}, nil
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
