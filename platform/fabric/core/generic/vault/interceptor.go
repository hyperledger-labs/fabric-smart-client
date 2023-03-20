/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

type Interceptor struct {
	QE        QueryExecutor
	TxIDStore TXIDStoreReader
	Rws       readWriteSet
	Closed    bool
	TxID      string
}

func NewInterceptor(qe QueryExecutor, txidStore TXIDStoreReader, txid string) *Interceptor {
	logger.Debugf("new interceptor [%s]", txid)

	return &Interceptor{
		TxID:      txid,
		QE:        qe,
		TxIDStore: txidStore,
		Rws: readWriteSet{
			readSet: readSet{
				orderedReads: map[string][]string{},
				reads:        reads{},
			},
			writeSet: writeSet{
				orderedWrites: map[string][]string{},
				writes:        writes{},
			},
			metaWriteSet: metaWriteSet{
				metawrites: namespaceKeyedMetaWrites{},
			},
		},
	}
}

func (i *Interceptor) IsValid() error {
	code, err := i.TxIDStore.Get(i.TxID)
	if err != nil {
		return err
	}
	if code == driver.Valid {
		return errors.Errorf("duplicate txid %s", i.TxID)
	}
	if i.QE != nil {

		for ns, nsMap := range i.Rws.reads {
			for k, v := range nsMap {
				_, b, t, err := i.QE.GetState(ns, k)
				if err != nil {
					return err
				}

				if b != v.block || t != v.txnum {
					return errors.Errorf("invalid read: vault at version %s:%s %d:%d, read-write set at version %d:%d", ns, k, b, t, v.block, v.txnum)
				}
			}
		}
	}
	return nil
}

func (i *Interceptor) Clear(ns string) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	i.Rws.readSet.clear(ns)
	i.Rws.writeSet.clear(ns)
	i.Rws.metaWriteSet.clear(ns)

	return nil
}

func (i *Interceptor) GetReadKeyAt(ns string, pos int) (string, error) {
	if i.Closed {
		return "", errors.New("this instance was closed")
	}

	key, in := i.Rws.readSet.getAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	return key, nil
}

func (i *Interceptor) GetReadAt(ns string, pos int) (string, []byte, error) {
	if i.Closed {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.Rws.readSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	val, err := i.GetState(ns, key, driver.FromStorage)
	if err != nil {
		return "", nil, err
	}

	return key, val, nil
}

func (i *Interceptor) GetWriteAt(ns string, pos int) (string, []byte, error) {
	if i.Closed {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.Rws.writeSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.Rws.writeSet.get(ns, key), nil
}

func (i *Interceptor) NumReads(ns string) int {
	return len(i.Rws.reads[ns])
}

func (i *Interceptor) NumWrites(ns string) int {
	return len(i.Rws.writes[ns])
}

func (i *Interceptor) Namespaces() []string {
	mergedMaps := map[string]struct{}{}

	for ns := range i.Rws.reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.Rws.writes {
		mergedMaps[ns] = struct{}{}
	}

	namespaces := make([]string, 0, len(mergedMaps))
	for ns := range mergedMaps {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

func (i *Interceptor) DeleteState(namespace string, key string) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	return i.SetState(namespace, key, nil)
}

func (i *Interceptor) SetState(namespace string, key string, value []byte) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}
	logger.Debugf("SetState [%s,%s,%s]", namespace, key, hash.Hashable(value).String())

	return i.Rws.writeSet.add(namespace, key, value)
}

func (i *Interceptor) SetStateMetadata(namespace string, key string, value map[string][]byte) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	return i.Rws.metaWriteSet.add(namespace, key, value)
}

func (i *Interceptor) GetStateMetadata(namespace, key string, opts ...driver.GetStateOpt) (map[string][]byte, error) {
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

		b, t, in := i.Rws.readSet.get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid metadata read: previous value returned at version %d:%d, current value at version %d:%d", b, t, block, txnum)
			}
		} else {
			i.Rws.readSet.add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.Rws.metaWriteSet.get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetStateMetadata(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.Rws.writeSet.in(namespace, key) {
			return val, err
		}

		return i.GetStateMetadata(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor) GetState(namespace string, key string, opts ...driver.GetStateOpt) ([]byte, error) {
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

		b, t, in := i.Rws.readSet.get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", namespace, key, b, t, block, txnum)
			}
		} else {
			i.Rws.readSet.add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.Rws.writeSet.get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetState(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.Rws.writeSet.in(namespace, key) {
			return val, err
		}

		return i.GetState(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor) AppendRWSet(raw []byte, nss ...string) error {
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

			b, t, in := i.Rws.readSet.get(ns, read.Key)
			if in && (b != bnum || t != txnum) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", ns, read.Key, b, t, b, txnum)
			}

			i.Rws.readSet.add(ns, read.Key, bnum, txnum)
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if i.Rws.writeSet.in(ns, write.Key) {
				return errors.Errorf("duplicate write entry for key %s:%s", ns, write.Key)
			}

			if err := i.Rws.writeSet.add(ns, write.Key, write.Value); err != nil {
				return err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			if i.Rws.metaWriteSet.in(ns, metaWrite.Key) {
				return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, metaWrite.Key)
			}

			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := i.Rws.metaWriteSet.add(ns, metaWrite.Key, metadata); err != nil {
				return err
			}
		}
	}

	return nil
}

func (i *Interceptor) Bytes() ([]byte, error) {
	rwsb := rwsetutil.NewRWSetBuilder()

	for ns, keyMap := range i.Rws.reads {
		for key, v := range keyMap {
			if v.block != 0 || v.txnum != 0 {
				rwsb.AddToReadSet(ns, key, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: v.block, TxNum: v.txnum}))
			} else {
				rwsb.AddToReadSet(ns, key, nil)
			}
		}
	}
	for ns, keyMap := range i.Rws.writes {
		for key, v := range keyMap {
			rwsb.AddToWriteSet(ns, key, v)
		}
	}
	for ns, keyMap := range i.Rws.metawrites {
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

func (i *Interceptor) Equals(other interface{}, nss ...string) error {
	switch o := other.(type) {
	case *Interceptor:
		if err := i.Rws.reads.equals(o.Rws.reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.Rws.writes.equals(o.Rws.writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.Rws.metawrites.equals(o.Rws.metawrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	case *Inspector:
		if err := i.Rws.reads.equals(o.Rws.reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.Rws.writes.equals(o.Rws.writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.Rws.metawrites.equals(o.Rws.metawrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	default:
		return errors.Errorf("cannot compare to the passed value [%v]", other)
	}
	return nil
}

func (i *Interceptor) Done() {
	logger.Debugf("Done with [%s], closed [%v]", i.TxID, i.Closed)
	if !i.Closed {
		i.Closed = true
		if i.QE != nil {
			i.QE.Done()
		}
	}
}

func (i *Interceptor) Reopen(qe QueryExecutor) error {
	logger.Debugf("Reopen with [%s], closed [%v]", i.TxID, i.Closed)
	if !i.Closed {
		return errors.Errorf("already open")
	}
	i.QE = qe
	i.Closed = false

	return nil
}
