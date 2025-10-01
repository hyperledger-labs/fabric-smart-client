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
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

type (
	Vault = vault.Vault[fdriver.ValidationCode]
)

// NewVault returns a new instance of Vault.
func NewVault(vaultStore vault2.CachedVaultStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) *Vault {
	m := NewMarshaller()

	interceptor := func(logger vault.Logger, ctx context.Context, rwSet vault.ReadWriteSet, qe vault.VersionedQueryExecutor, vaultStore vault.TxStatusStore, txID driver.TxID) vault.TxInterceptor {
		logger.Debugf("create new interceptor for [txID=%v]", txID)
		ci := vault.NewInterceptor(logger, ctx, rwSet, qe, vaultStore, txID, fdriver.ValidationCodeProvider, nil, &CounterBasedVersionComparator{})
		return newInterceptor(ci, txID, qe, m)
	}

	return vault.New(
		logging.MustGetLogger(),
		vaultStore,
		fdriver.ValidationCodeProvider,
		interceptor,
		&populator{marshaller: m},
		metricsProvider,
		tracerProvider,
		&CounterBasedVersionBuilder{},
	)
}

type populator struct {
	marshaller *Marshaller
}

func (p *populator) Populate(rwsetBytes []byte, namespaces ...driver.Namespace) (vault.ReadWriteSet, error) {
	rws, err := p.marshaller.RWSetFromBytes(rwsetBytes, namespaces...)
	if err != nil {
		return vault.ReadWriteSet{}, err
	}
	return *rws, nil
}
