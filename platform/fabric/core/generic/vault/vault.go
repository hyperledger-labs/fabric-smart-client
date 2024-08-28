/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type (
	TXIDStore       = vault.TXIDStore[fdriver.ValidationCode]
	Vault           = vault.Vault[fdriver.ValidationCode]
	TXIDStoreReader = vault.TXIDStoreReader[fdriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[fdriver.ValidationCode]
)

// NewTXIDStore returns a new instance of SimpleTXIDStore
func NewTXIDStore(persistence txidstore.UnversionedPersistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[fdriver.ValidationCode](
		persistence,
		&fdriver.ValidationCodeProvider{},
	)
}

// NewVault returns a new instance of Vault
func NewVault(store vault.VersionedPersistence, txIDStore TXIDStore, tracerProvider trace.TracerProvider) *Vault {
	return vault.New[fdriver.ValidationCode](
		flogging.MustGetLogger("fabric-sdk.generic.vault"),
		store,
		txIDStore,
		&fdriver.ValidationCodeProvider{},
		newInterceptor,
		&populator{},
		tracerProvider,
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

type populator struct{}

func (p *populator) Populate(rws *vault.ReadWriteSet, rwsetBytes []byte, namespaces ...driver.Namespace) error {
	txRWSet := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(rwsetBytes, txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	rwsIn, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes, TxRwSetFromProtoMsg failed")
	}

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
			rws.ReadSet.Add(ns, read.Key, bn, txn)
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if err := rws.WriteSet.Add(ns, write.Key, write.Value); err != nil {
				return err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := rws.MetaWriteSet.Add(ns, metaWrite.Key, metadata); err != nil {
				return err
			}
		}
	}

	return nil
}

type marshaller struct{}

func (m *marshaller) Marshal(rws *vault.ReadWriteSet) ([]byte, error) {
	rwsb := rwsetutil.NewRWSetBuilder()

	for ns, keyMap := range rws.Reads {
		for key, v := range keyMap {
			if v.Block != 0 || v.TxNum != 0 {
				rwsb.AddToReadSet(ns, key, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: v.Block, TxNum: v.TxNum}))
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

	return simRes.GetPubSimulationBytes()
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

			b, t, in := destination.ReadSet.Get(ns, read.Key)
			if in && (b != bnum || t != txnum) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", ns, read.Key, b, t, b, txnum)
			}

			destination.ReadSet.Add(ns, read.Key, bnum, txnum)
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
