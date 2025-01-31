/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type Vault = vault.Vault[odriver.ValidationCode]

// New returns a new instance of Vault
func New(vaultStore driver.VaultStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) *Vault {
	return vault.New[odriver.ValidationCode](
		logging.MustGetLogger("orion-sdk.generic.vault"),
		vault2.NewCachedVault(vaultStore, 0),
		odriver.ValidationCodeProvider,
		newInterceptor,
		&populator{},
		metricsProvider,
		tracerProvider,
		&vault.BlockTxIndexVersionBuilder{},
	)
}

type Interceptor struct {
	*vault.Interceptor[odriver.ValidationCode]
}

func newInterceptor(
	logger vault.Logger,
	ctx context.Context,
	rwSet vault.ReadWriteSet,
	qe vault.VersionedQueryExecutor,
	vaultStore vault.TxStatusStore,
	txid string,
) vault.TxInterceptor {
	return &Interceptor{Interceptor: vault.NewInterceptor[odriver.ValidationCode](
		logger,
		ctx,
		rwSet,
		qe,
		vaultStore,
		txid,
		odriver.ValidationCodeProvider,
		nil,
		&vault.BlockTxIndexVersionComparator{},
	)}
}

func (i *Interceptor) AppendRWSet([]byte, ...string) error {
	if i.Interceptor.IsClosed() {
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

type populator struct {
	versionMarshaller vault.BlockTxIndexVersionMarshaller
}

func (p *populator) Populate(rwsetBytes []byte, namespaces ...driver.Namespace) (vault.ReadWriteSet, error) {
	rws := vault.EmptyRWSet()
	txRWSet := &types.DataTx{}
	err := proto.Unmarshal(rwsetBytes, txRWSet)
	if err != nil {
		return vault.ReadWriteSet{}, errors.Wrapf(err, "provided invalid read-write set bytes, unmarshal failed")
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
				p.versionMarshaller.ToBytes(
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
				return vault.ReadWriteSet{}, errors.Wrapf(err, "failed to add write to read-write set")
			}
			// TODO: What about write.ACL? Shall we store it as metadata?
		}

		for _, del := range operation.DataDeletes {
			if err := rws.WriteSet.Add(
				operation.DbName,
				del.Key,
				nil,
			); err != nil {
				return vault.ReadWriteSet{}, errors.Wrapf(err, "failed to add delete to read-write set")
			}
		}
	}

	return rws, nil
}
