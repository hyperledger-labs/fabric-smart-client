/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

type (
	TXIDStore       = vault.TXIDStore[fdriver.ValidationCode]
	Vault           = vault.Vault[fdriver.ValidationCode]
	TXIDStoreReader = vault.TXIDStoreReader[fdriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[fdriver.ValidationCode]
)

// NewVault returns a new instance of Vault
func NewVault(store vault.VersionedPersistence, txIDStore TXIDStore) *Vault {
	return vault.New[fdriver.ValidationCode](
		flogging.MustGetLogger("fabric-sdk.generic.vault"),
		store,
		txIDStore,
		&fdriver.ValidationCodeProvider{},
		newInterceptor,
		&populator{},
	)
}

func newInterceptor(logger vault.Logger, qe vault.VersionedQueryExecutor, txIDStore TXIDStoreReader, txID string) vault.TxInterceptor {
	return vault.NewInterceptor[fdriver.ValidationCode](
		logger,
		qe,
		txIDStore,
		txID,
		&fdriver.ValidationCodeProvider{},
		&marshaller{},
	)
}

// populator is the custom populator for FabricDEV
type populator struct{}

func (p *populator) Populate(rws *vault.ReadWriteSet, rwsetBytes []byte, namespaces ...driver.Namespace) error {
	panic("implement me")
}

// marshaller is the custom marshaller for FabricDEV
type marshaller struct{}

func (m *marshaller) Marshal(rws *vault.ReadWriteSet) ([]byte, error) {
	panic("implement me")
}

func (m *marshaller) Append(destination *vault.ReadWriteSet, raw []byte, nss ...string) error {
	panic("implement me")
}
