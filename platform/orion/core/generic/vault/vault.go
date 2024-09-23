/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/fver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type (
	TXIDStoreReader = vault.TXIDStoreReader[odriver.ValidationCode]
	Vault           = vault.Vault[odriver.ValidationCode]
	TXIDStore       = vault.TXIDStore[odriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[odriver.ValidationCode]
)

func NewSimpleTXIDStore(persistence txidstore.UnversionedPersistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[odriver.ValidationCode](persistence, &odriver.ValidationCodeProvider{})
}

// New returns a new instance of Vault
func New(store vault.VersionedPersistence, txIDStore TXIDStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) *Vault {
	return vault.New[odriver.ValidationCode](
		flogging.MustGetLogger("orion-sdk.generic.vault"),
		store,
		txIDStore,
		&odriver.ValidationCodeProvider{},
		newInterceptor,
		&populator{},
		metricsProvider,
		tracerProvider,
	)
}

type Interceptor struct {
	*vault.Interceptor[odriver.ValidationCode]
}

func newInterceptor(
	logger vault.Logger,
	qe vault.VersionedQueryExecutor,
	txidStore vault.TXIDStoreReader[odriver.ValidationCode],
	txid string,
) vault.TxInterceptor {
	return &Interceptor{Interceptor: vault.NewInterceptor[odriver.ValidationCode](
		logger,
		qe,
		txidStore,
		txid,
		&odriver.ValidationCodeProvider{},
		nil,
	)}
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

type populator struct{}

func (p *populator) Populate(rws *vault.ReadWriteSet, rwsetBytes []byte, namespaces ...driver.Namespace) error {
	txRWSet := &types.DataTx{}
	err := proto.Unmarshal(rwsetBytes, txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	for _, operation := range txRWSet.DbOperations {

		for _, read := range operation.DataReads {
			bn := uint64(0)
			txn := uint64(0)
			if read.Version != nil {
				bn = read.Version.BlockNum
				txn = read.Version.TxNum
			}
			rws.ReadSet.Add(
				operation.DbName,
				read.Key,
				fver.ToBytes(
					bn,
					txn,
				),
			)
		}

		for _, write := range operation.DataWrites {
			if err := rws.WriteSet.Add(
				operation.DbName,
				write.Key,
				write.Value,
			); err != nil {
				return errors.Wrapf(err, "failed to add write to read-write set")
			}
			// TODO: What about write.ACL? Shall we store it as metadata?
		}

		for _, del := range operation.DataDeletes {
			if err := rws.WriteSet.Add(
				operation.DbName,
				del.Key,
				nil,
			); err != nil {
				return errors.Wrapf(err, "failed to add delete to read-write set")
			}
		}
	}

	return nil
}
