/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
)

type QueryExecutor interface {
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	GetState(namespace, key string) ([]byte, uint64, uint64, error)
	Done()
}

type Interceptor[V ValidationCode] struct {
	QE         QueryExecutor
	TxIDStore  TXIDStoreReader[V]
	Rws        ReadWriteSet
	Closed     bool
	TxID       string
	vcProvider ValidationCodeProvider[V] // TODO
}

func NewInterceptor[V ValidationCode](qe QueryExecutor, txidStore TXIDStoreReader[V], txid core.TxID, vcProvider ValidationCodeProvider[V]) *Interceptor[V] {
	logger.Debugf("new interceptor [%s]", txid)

	return &Interceptor[V]{
		TxID:      txid,
		QE:        qe,
		TxIDStore: txidStore,
		Rws: ReadWriteSet{
			ReadSet: ReadSet{
				OrderedReads: map[string][]string{},
				Reads:        Reads{},
			},
			WriteSet: WriteSet{
				OrderedWrites: map[string][]string{},
				Writes:        Writes{},
			},
			MetaWriteSet: MetaWriteSet{
				MetaWrites: NamespaceKeyedMetaWrites{},
			},
		},
		vcProvider: vcProvider,
	}
}

func (i *Interceptor[V]) IsValid() error {
	code, _, err := i.TxIDStore.Get(i.TxID)
	if err != nil {
		return err
	}
	if code == i.vcProvider.Valid() {
		return errors.Errorf("duplicate txid %s", i.TxID)
	}
	if i.QE != nil {

		for ns, nsMap := range i.Rws.Reads {
			for k, v := range nsMap {
				_, b, t, err := i.QE.GetState(ns, k)
				if err != nil {
					return err
				}

				if b != v.Block || t != v.TxNum {
					return errors.Errorf("invalid read: vault at version %s:%s %d:%d, read-write set at version %d:%d", ns, k, b, t, v.Block, v.TxNum)
				}
			}
		}
	}
	return nil
}

func (i *Interceptor[V]) Clear(ns string) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	i.Rws.ReadSet.clear(ns)
	i.Rws.WriteSet.clear(ns)
	i.Rws.MetaWriteSet.clear(ns)

	return nil
}

func (i *Interceptor[V]) GetReadKeyAt(ns string, pos int) (string, error) {
	if i.Closed {
		return "", errors.New("this instance was closed")
	}

	key, in := i.Rws.ReadSet.getAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	return key, nil
}

func (i *Interceptor[V]) GetReadAt(ns string, pos int) (string, []byte, error) {
	if i.Closed {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.Rws.ReadSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	val, err := i.GetState(ns, key, driver.FromStorage)
	if err != nil {
		return "", nil, err
	}

	return key, val, nil
}

func (i *Interceptor[V]) GetWriteAt(ns string, pos int) (string, []byte, error) {
	if i.Closed {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.Rws.WriteSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.Rws.WriteSet.get(ns, key), nil
}

func (i *Interceptor[V]) NumReads(ns string) int {
	return len(i.Rws.Reads[ns])
}

func (i *Interceptor[V]) NumWrites(ns string) int {
	return len(i.Rws.Writes[ns])
}

func (i *Interceptor[V]) Namespaces() []string {
	mergedMaps := map[string]struct{}{}

	for ns := range i.Rws.Reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.Rws.Writes {
		mergedMaps[ns] = struct{}{}
	}

	namespaces := make([]string, 0, len(mergedMaps))
	for ns := range mergedMaps {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

func (i *Interceptor[V]) DeleteState(namespace string, key string) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	return i.SetState(namespace, key, nil)
}

func (i *Interceptor[V]) SetState(namespace string, key string, value []byte) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}
	logger.Debugf("SetState [%s,%s,%s]", namespace, key, hash.Hashable(value).String())

	return i.Rws.WriteSet.add(namespace, key, value)
}

func (i *Interceptor[V]) SetStateMetadata(namespace string, key string, value map[string][]byte) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	return i.Rws.MetaWriteSet.add(namespace, key, value)
}

func (i *Interceptor[V]) GetStateMetadata(namespace, key string, opts ...driver.GetStateOpt) (map[string][]byte, error) {
	if i.Closed {
		return nil, errors.New("this instance was closed")
	}

	if i.QE == nil {
		return nil, errors.New("this instance is write only")
	}

	if len(opts) > 1 {
		return nil, errors.Errorf("a single getoption is supported, %d provided", len(opts))
	}

	opt := driver.FromStorage
	if len(opts) == 1 {
		opt = opts[0]
	}

	switch opt {
	case driver.FromStorage:
		val, block, txnum, err := i.QE.GetStateMetadata(namespace, key)
		if err != nil {
			return nil, err
		}

		b, t, in := i.Rws.ReadSet.get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid metadata read: previous value returned at version %d:%d, current value at version %d:%d", b, t, block, txnum)
			}
		} else {
			i.Rws.ReadSet.add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.Rws.MetaWriteSet.get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetStateMetadata(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.Rws.WriteSet.in(namespace, key) {
			return val, err
		}

		return i.GetStateMetadata(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor[V]) GetState(namespace string, key string, opts ...driver.GetStateOpt) ([]byte, error) {
	if i.Closed {
		return nil, errors.New("this instance was closed")
	}

	if i.QE == nil {
		return nil, errors.New("this instance is write only")
	}

	if len(opts) > 1 {
		return nil, errors.Errorf("a single getoption is supported, %d provided", len(opts))
	}

	opt := driver.FromStorage
	if len(opts) == 1 {
		opt = opts[0]
	}

	switch opt {
	case driver.FromStorage:
		val, block, txnum, err := i.QE.GetState(namespace, key)
		if err != nil {
			return nil, err
		}

		b, t, in := i.Rws.ReadSet.get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", namespace, key, b, t, block, txnum)
			}
		} else {
			i.Rws.ReadSet.add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.Rws.WriteSet.get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetState(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.Rws.WriteSet.in(namespace, key) {
			return val, err
		}

		return i.GetState(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor[V]) AppendRWSet(raw []byte, nss ...string) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	txRWSet := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(raw, txRWSet)
	if err != nil {
		return errors.Wrap(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	rws, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	if err != nil {
		return errors.Wrap(err, "provided invalid read-write set bytes, TxRwSetFromProtoMsg failed")
	}

	for _, nsrws := range rws.NsRwSets {
		ns := nsrws.NameSpace
		if len(nss) != 0 {
			found := false
			for _, ref := range nss {
				if ns == ref {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		for _, read := range nsrws.KvRwSet.Reads {
			bnum := uint64(0)
			txnum := uint64(0)
			if read.Version != nil {
				bnum = read.Version.BlockNum
				txnum = read.Version.TxNum
			}

			b, t, in := i.Rws.ReadSet.get(ns, read.Key)
			if in && (b != bnum || t != txnum) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", ns, read.Key, b, t, b, txnum)
			}

			i.Rws.ReadSet.add(ns, read.Key, bnum, txnum)
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if i.Rws.WriteSet.in(ns, write.Key) {
				return errors.Errorf("duplicate write entry for key %s:%s", ns, write.Key)
			}

			if err := i.Rws.WriteSet.add(ns, write.Key, write.Value); err != nil {
				return err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			if i.Rws.MetaWriteSet.in(ns, metaWrite.Key) {
				return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, metaWrite.Key)
			}

			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := i.Rws.MetaWriteSet.add(ns, metaWrite.Key, metadata); err != nil {
				return err
			}
		}
	}

	return nil
}

func (i *Interceptor[V]) Bytes() ([]byte, error) {
	rwsb := rwsetutil.NewRWSetBuilder()

	for ns, keyMap := range i.Rws.Reads {
		for key, v := range keyMap {
			if v.Block != 0 || v.TxNum != 0 {
				rwsb.AddToReadSet(ns, key, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: v.Block, TxNum: v.TxNum}))
			} else {
				rwsb.AddToReadSet(ns, key, nil)
			}
		}
	}
	for ns, keyMap := range i.Rws.Writes {
		for key, v := range keyMap {
			rwsb.AddToWriteSet(ns, key, v)
		}
	}
	for ns, keyMap := range i.Rws.MetaWrites {
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

func (i *Interceptor[V]) Equals(other interface{}, nss ...string) error {
	switch o := other.(type) {
	case *Interceptor[V]:
		if err := i.Rws.Reads.equals(o.Rws.Reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.Rws.Writes.equals(o.Rws.Writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.Rws.MetaWrites.equals(o.Rws.MetaWrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	case *Inspector:
		if err := i.Rws.Reads.equals(o.Rws.Reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.Rws.Writes.equals(o.Rws.Writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.Rws.MetaWrites.equals(o.Rws.MetaWrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	default:
		return errors.Errorf("cannot compare to the passed value [%v]", other)
	}
	return nil
}

func (i *Interceptor[V]) Done() {
	logger.Debugf("Done with [%s], closed [%v]", i.TxID, i.Closed)
	if !i.Closed {
		i.Closed = true
		if i.QE != nil {
			i.QE.Done()
		}
	}
}

func (i *Interceptor[V]) Reopen(qe QueryExecutor) error {
	logger.Debugf("Reopen with [%s], closed [%v]", i.TxID, i.Closed)
	if !i.Closed {
		return errors.Errorf("already open")
	}
	i.QE = qe
	i.Closed = false

	return nil
}

func (i *Interceptor[V]) IsClosed() bool {
	return i.Closed
}
func (i *Interceptor[V]) RWs() *ReadWriteSet {
	return &i.Rws
}
