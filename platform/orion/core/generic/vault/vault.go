/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

type (
	TXIDStoreReader = vault.TXIDStoreReader[odriver.ValidationCode]
	Vault           = vault.Vault[odriver.ValidationCode]
	TXIDStore       = vault.TXIDStore[odriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[odriver.ValidationCode]
)

func NewSimpleTXIDStore(persistence driver.Persistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[odriver.ValidationCode](persistence, &odriver.ValidationCodeProvider{})
}

// New returns a new instance of Vault
func New(store driver.VersionedPersistence, txIDStore TXIDStore) *Vault {
	return vault.New[odriver.ValidationCode](store, txIDStore, &odriver.ValidationCodeProvider{}, newInterceptor)
}

type Interceptor struct {
	*vault.Interceptor[odriver.ValidationCode]
}

func newInterceptor(qe vault.QueryExecutor, txidStore vault.TXIDStoreReader[odriver.ValidationCode], txid string) vault.TxInterceptor {
	return &Interceptor{Interceptor: vault.NewInterceptor[odriver.ValidationCode](qe, txidStore, txid, &odriver.ValidationCodeProvider{})}
}

func (i *Interceptor) AppendRWSet([]byte, ...string) error {
	if i.Interceptor.Closed {
		return errors.New("this instance was closed")
	}
	return nil
}

func (i *Interceptor) Bytes() ([]byte, error) {
	panic("")
}

func (i *Interceptor) Equals(other interface{}, nss ...string) error {
	if _, ok := other.(*Interceptor); ok {
		return i.Interceptor.Equals(other, nss...)
	}
	return errors.Errorf("cannot compare to the passed value [%v]", other)
}
