/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	vault3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

// NewVault returns a new instance of Vault
func NewVault(vaultStore vault2.CachedVaultStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) *vault3.Vault {
	return vault.New[fdriver.ValidationCode](
		logging.MustGetLogger(),
		vaultStore,
		fdriver.ValidationCodeProvider,
		newInterceptor,
		&populator{},
		metricsProvider,
		tracerProvider,
		&CounterBasedVersionBuilder{},
	)
}

func newInterceptor(logger vault.Logger, ctx context.Context, rwSet vault.ReadWriteSet, qe vault.VersionedQueryExecutor, vaultStore vault.TxStatusStore, txID string) vault.TxInterceptor {
	return vault.NewInterceptor[fdriver.ValidationCode](
		logger,
		ctx,
		rwSet,
		qe,
		vaultStore,
		txID,
		fdriver.ValidationCodeProvider,
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
