/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
)

type (
	TXIDStore       = vault.TXIDStore[fdriver.ValidationCode]
	Vault           = vault.Vault[fdriver.ValidationCode]
	TXIDStoreReader = vault.TXIDStoreReader[fdriver.ValidationCode]
	SimpleTXIDStore = txidstore.SimpleTXIDStore[fdriver.ValidationCode]
)

func NewTXIDStore(persistence txidstore.UnversionedPersistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[fdriver.ValidationCode](persistence, &fdriver.ValidationCodeProvider{})
}

// New returns a new instance of Vault
func New(store vault.VersionedPersistence, txIDStore TXIDStore) *Vault {
	return vault.New[fdriver.ValidationCode](flogging.MustGetLogger("fabric-sdk.generic.vault"), store, txIDStore, &fdriver.ValidationCodeProvider{}, newInterceptor, &populator{})
}

func newInterceptor(logger vault.Logger, qe vault.VersionedQueryExecutor, txidStore TXIDStoreReader, txid string) vault.TxInterceptor {
	return vault.NewInterceptor[fdriver.ValidationCode](logger, qe, txidStore, txid, &fdriver.ValidationCodeProvider{})
}

type populator struct{}

func (p *populator) Populate(rws *vault.ReadWriteSet, rwsetBytes []byte, namespaces ...core.Namespace) error {
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
			bn := core.BlockNum(0)
			txn := core.TxNum(0)
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
