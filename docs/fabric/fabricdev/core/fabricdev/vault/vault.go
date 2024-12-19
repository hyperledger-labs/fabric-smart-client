/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

type (
	TXIDStore       = vault.TXIDStore[fdriver.ValidationCode]
	Vault           = vault.Vault[fdriver.ValidationCode]
	TXIDStoreReader = vault.TXIDStoreReader[fdriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[fdriver.ValidationCode]
)

// NewVault returns a new instance of Vault
func NewVault(store vault.VersionedPersistence, txIDStore TXIDStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) *Vault {
	return vault.New[fdriver.ValidationCode](
		logging.MustGetLogger("fabric-sdk.generic.vault"),
		store,
		txIDStore,
		&fdriver.ValidationCodeProvider{},
		newInterceptor,
		&populator{},
		metricsProvider,
		tracerProvider,
		&CounterBasedVersionBuilder{},
	)
}

func newInterceptor(logger vault.Logger, rwSet vault.ReadWriteSet, qe vault.VersionedQueryExecutor, txIDStore TXIDStoreReader, txID string) vault.TxInterceptor {
	return vault.NewInterceptor[fdriver.ValidationCode](
		logger,
		rwSet,
		qe,
		txIDStore,
		txID,
		&fdriver.ValidationCodeProvider{},
		&marshaller{},
		&CounterBasedVersionComparator{},
	)
}

// populator is the custom populator for FabricDEV
type populator struct{}

func (p *populator) Populate([]byte, ...driver.Namespace) (vault.ReadWriteSet, error) {
	panic("implement me")
}

// marshaller is the custom marshaller for FabricDEV
type marshaller struct{}

func (m *marshaller) Marshal(txID string, rws *vault.ReadWriteSet) ([]byte, error) {
	panic("implement me")
}

func (m *marshaller) Append(destination *vault.ReadWriteSet, raw []byte, nss ...string) error {
	panic("implement me")
}
