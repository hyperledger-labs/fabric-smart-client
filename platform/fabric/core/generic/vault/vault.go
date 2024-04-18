/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type (
	TXIDStore       = vault.TXIDStore[fdriver.ValidationCode]
	Vault           = vault.Vault[fdriver.ValidationCode]
	TXIDStoreReader = vault.TXIDStoreReader[fdriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[fdriver.ValidationCode]
)

func NewTXIDStore(persistence driver.Persistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[fdriver.ValidationCode](persistence, &fdriver.ValidationCodeProvider{})
}

// New returns a new instance of Vault
func New(store driver.VersionedPersistence, txIDStore TXIDStore) *Vault {
	return vault.New[fdriver.ValidationCode](store, txIDStore, &fdriver.ValidationCodeProvider{}, newInterceptor)
}

func newInterceptor(qe vault.QueryExecutor, txidStore TXIDStoreReader, txid string) vault.TxInterceptor {
	return vault.NewInterceptor[fdriver.ValidationCode](qe, txidStore, txid, &fdriver.ValidationCodeProvider{})
}
