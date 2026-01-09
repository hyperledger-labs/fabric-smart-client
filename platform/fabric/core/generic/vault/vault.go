/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/kvledger/rwsetutil"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
)

type Vault = vault.Vault[fdriver.ValidationCode]

// NewVault returns a new instance of Vault
func NewVault(vaultStore vault2.CachedVaultStore, metricsProvider metrics.Provider, tracerProvider tracing.Provider) *Vault {
	return vault.New[fdriver.ValidationCode](
		logging.MustGetLogger(),
		vaultStore,
		fdriver.ValidationCodeProvider,
		newInterceptor,
		NewPopulator(),
		metricsProvider,
		tracerProvider,
		&vault.BlockTxIndexVersionBuilder{},
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
		&vault.BlockTxIndexVersionComparator{},
	)
}

type populator struct {
	versionMarshaller vault.BlockTxIndexVersionMarshaller
}

func NewPopulator() *populator {
	return &populator{}
}

func (p *populator) Populate(rwsetBytes []byte, namespaces ...driver.Namespace) (vault.ReadWriteSet, error) {
	txRWSet := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(rwsetBytes, txRWSet)
	if err != nil {
		return vault.ReadWriteSet{}, errors.Wrapf(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	rwsIn, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	if err != nil {
		return vault.ReadWriteSet{}, errors.Wrapf(err, "provided invalid read-write set bytes, TxRwSetFromProtoMsg failed")
	}

	rws := vault.EmptyRWSet()
	namespaceSet := collections.NewSet(namespaces...)
	for _, nsrws := range rwsIn.NsRwSets {
		ns := nsrws.NameSpace

		// skip if not in the list of namespaces
		if !namespaceSet.Empty() && !namespaceSet.Contains(ns) {
			continue
		}

		for _, read := range nsrws.KvRwSet.Reads {
			bn := driver.BlockNum(0)
			txn := driver.TxNum(0)
			if read.Version != nil {
				bn = read.Version.BlockNum
				txn = read.Version.TxNum
			}
			rws.ReadSet.Add(ns, read.Key, p.versionMarshaller.ToBytes(bn, txn))
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if err := rws.WriteSet.Add(ns, write.Key, write.Value); err != nil {
				return vault.ReadWriteSet{}, err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := rws.MetaWriteSet.Add(ns, metaWrite.Key, metadata); err != nil {
				return vault.ReadWriteSet{}, err
			}
		}
	}

	return rws, nil
}

type marshaller struct {
	versionMarshaller vault.BlockTxIndexVersionMarshaller
}

func (m *marshaller) Marshal(txID string, rws *vault.ReadWriteSet) ([]byte, error) {
	rwsb := rwsetutil.NewRWSetBuilder()

	for ns, keyMap := range rws.Reads {
		for key, v := range keyMap {
			block, txNum, err := m.versionMarshaller.FromBytes(v)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to extract block version from bytes [%v]", v)
			}
			if block != 0 || txNum != 0 {
				rwsb.AddToReadSet(ns, key, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: block, TxNum: txNum}))
			} else {
				rwsb.AddToReadSet(ns, key, nil)
			}
		}
	}
	for ns, keyMap := range rws.Writes {
		for key, v := range keyMap {
			rwsb.AddToWriteSet(ns, key, v)
		}
	}
	for ns, keyMap := range rws.MetaWrites {
		for key, v := range keyMap {
			rwsb.AddToMetadataWriteSet(ns, key, v)
		}
	}

	simRes, err := rwsb.GetTxSimulationResults()
	if err != nil {
		return nil, err
	}

	return proto.Marshal(simRes.PubSimulationResults)
}

func (m *marshaller) Append(destination *vault.ReadWriteSet, raw []byte, nss ...string) error {
	txRWSet := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(raw, txRWSet)
	if err != nil {
		return errors.Wrap(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	source, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	if err != nil {
		return errors.Wrap(err, "provided invalid read-write set bytes, TxRwSetFromProtoMsg failed")
	}

	namespaces := collections.NewSet(nss...)
	for _, nsrws := range source.NsRwSets {
		ns := nsrws.NameSpace
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}

		for _, read := range nsrws.KvRwSet.Reads {
			bnum := fdriver.BlockNum(0)
			txnum := fdriver.TxNum(0)
			if read.Version != nil {
				bnum = read.Version.BlockNum
				txnum = read.Version.TxNum
			}
			dVersion, in := destination.ReadSet.Get(ns, read.Key)
			b, t, err := m.versionMarshaller.FromBytes(dVersion)
			if err != nil {
				return errors.Wrapf(err, "failed to extract block version from bytes [%v]", dVersion)
			}
			if in && (b != bnum || t != txnum) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at version [%v], current value at version [%v]", ns, read.Key, m.versionMarshaller.ToBytes(bnum, txnum), dVersion)
			}
			destination.ReadSet.Add(ns, read.Key, dVersion)
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if destination.WriteSet.In(ns, write.Key) {
				return errors.Errorf("duplicate write entry for key %s:%s", ns, write.Key)
			}

			if err := destination.WriteSet.Add(ns, write.Key, write.Value); err != nil {
				return err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			if destination.MetaWriteSet.In(ns, metaWrite.Key) {
				return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, metaWrite.Key)
			}

			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := destination.MetaWriteSet.Add(ns, metaWrite.Key, metadata); err != nil {
				return err
			}
		}
	}

	return nil
}
