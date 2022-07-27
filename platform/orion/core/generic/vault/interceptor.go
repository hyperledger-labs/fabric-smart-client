/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type QueryExecutor interface {
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	GetState(namespace, key string) ([]byte, uint64, uint64, error)
	Done()
}

type Interceptor struct {
	qe        QueryExecutor
	txidStore TXIDStoreReader
	rws       readWriteSet
	closed    bool
	txid      string
	dbName    string
}

func newInterceptor(qe QueryExecutor, txidStore TXIDStoreReader, txid string) *Interceptor {
	logger.Debugf("new interceptor [%s]", txid)

	return &Interceptor{
		txid:      txid,
		qe:        qe,
		txidStore: txidStore,
		rws: readWriteSet{
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
	code, err := i.txidStore.Get(i.txid)
	if err != nil {
		return err
	}
	if code == driver.Valid {
		return errors.Errorf("duplicate txid %s", i.txid)
	}

	for ns, nsMap := range i.rws.reads {
		for k, v := range nsMap {
			_, b, t, err := i.qe.GetState(ns, k)
			if err != nil {
				return err
			}

			if b != v.block || t != v.txnum {
				return errors.Errorf("invalid read: vault at version %s:%s %d:%d, read-write set at version %d:%d", ns, k, b, t, v.block, v.txnum)
			}
		}
	}

	return nil
}

func (i *Interceptor) Clear(ns string) error {
	if i.closed {
		return errors.New("this instance was closed")
	}

	i.rws.readSet.clear(ns)
	i.rws.writeSet.clear(ns)
	i.rws.metaWriteSet.clear(ns)

	return nil
}

func (i *Interceptor) GetReadKeyAt(ns string, pos int) (string, error) {
	if i.closed {
		return "", errors.New("this instance was closed")
	}

	key, in := i.rws.readSet.getAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	return key, nil
}

func (i *Interceptor) GetReadAt(ns string, pos int) (string, []byte, error) {
	if i.closed {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.rws.readSet.getAt(ns, pos)
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
	if i.closed {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.rws.writeSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.rws.writeSet.get(ns, key), nil
}

func (i *Interceptor) NumReads(ns string) int {
	return len(i.rws.reads[ns])
}

func (i *Interceptor) NumWrites(ns string) int {
	return len(i.rws.writes[ns])
}

func (i *Interceptor) Namespaces() []string {
	mergedMaps := map[string]struct{}{}

	for ns := range i.rws.reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.rws.writes {
		mergedMaps[ns] = struct{}{}
	}

	namespaces := make([]string, 0, len(mergedMaps))
	for ns := range mergedMaps {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

func (i *Interceptor) DeleteState(namespace string, key string) error {
	if i.closed {
		return errors.New("this instance was closed")
	}

	return i.SetState(namespace, key, nil)
}

func (i *Interceptor) SetState(namespace string, key string, value []byte) error {
	if i.closed {
		return errors.New("this instance was closed")
	}
	logger.Debugf("SetState [%s,%s,%s]", namespace, key, hash.Hashable(value).String())

	return i.rws.writeSet.add(namespace, key, value)
}

func (i *Interceptor) SetStateMetadata(namespace string, key string, value map[string][]byte) error {
	if i.closed {
		return errors.New("this instance was closed")
	}

	return i.rws.metaWriteSet.add(namespace, key, value)
}

func (i *Interceptor) GetStateMetadata(namespace, key string, opts ...driver.GetStateOpt) (map[string][]byte, error) {
	if i.closed {
		return nil, errors.New("this instance was closed")
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
		val, block, txnum, err := i.qe.GetStateMetadata(namespace, key)
		if err != nil {
			return nil, err
		}

		b, t, in := i.rws.readSet.get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid metadata read: previous value returned at version %d:%d, current value at version %d:%d", b, t, block, txnum)
			}
		} else {
			i.rws.readSet.add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.rws.metaWriteSet.get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetStateMetadata(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.rws.writeSet.in(namespace, key) {
			return val, err
		}

		return i.GetStateMetadata(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor) GetState(namespace string, key string, opts ...driver.GetStateOpt) ([]byte, error) {
	if i.closed {
		return nil, errors.New("this instance was closed")
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
		val, block, txnum, err := i.qe.GetState(namespace, key)
		if err != nil {
			return nil, err
		}

		b, t, in := i.rws.readSet.get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", namespace, key, b, t, block, txnum)
			}
		} else {
			i.rws.readSet.add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.rws.writeSet.get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetState(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.rws.writeSet.in(namespace, key) {
			return val, err
		}

		return i.GetState(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor) AppendRWSet(raw []byte, nss ...string) error {
	if i.closed {
		return errors.New("this instance was closed")
	}

	// txRWSet := &rwset.TxReadWriteSet{}
	// err := proto.Unmarshal(raw, txRWSet)
	// if err != nil {
	// 	return errors.Wrap(err, "provided invalid read-write set bytes, unmarshal failed")
	// }
	//
	// rws, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	// if err != nil {
	// 	return errors.Wrap(err, "provided invalid read-write set bytes, TxRwSetFromProtoMsg failed")
	// }
	//
	// for _, nsrws := range rws.NsRwSets {
	// 	ns := nsrws.NameSpace
	// 	if len(nss) != 0 {
	// 		found := false
	// 		for _, ref := range nss {
	// 			if ns == ref {
	// 				found = true
	// 				break
	// 			}
	// 		}
	// 		if !found {
	// 			continue
	// 		}
	// 	}
	//
	// 	for _, read := range nsrws.KvRwSet.Reads {
	// 		bnum := uint64(0)
	// 		txnum := uint64(0)
	// 		if read.Version != nil {
	// 			bnum = read.Version.BlockNum
	// 			txnum = read.Version.TxNum
	// 		}
	//
	// 		b, t, in := i.rws.readSet.get(ns, read.Key)
	// 		if in && (b != bnum || t != txnum) {
	// 			return errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", ns, read.Key, b, t, b, txnum)
	// 		}
	//
	// 		i.rws.readSet.add(ns, read.Key, bnum, txnum)
	// 	}
	//
	// 	for _, write := range nsrws.KvRwSet.Writes {
	// 		if i.rws.writeSet.in(ns, write.Key) {
	// 			return errors.Errorf("duplicate write entry for key %s:%s", ns, write.Key)
	// 		}
	//
	// 		if err := i.rws.writeSet.add(ns, write.Key, write.Value); err != nil {
	// 			return err
	// 		}
	// 	}
	//
	// 	for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
	// 		if i.rws.metaWriteSet.in(ns, metaWrite.Key) {
	// 			return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, metaWrite.Key)
	// 		}
	//
	// 		metadata := map[string][]byte{}
	// 		for _, entry := range metaWrite.Entries {
	// 			metadata[entry.Name] = append([]byte(nil), entry.Value...)
	// 		}
	//
	// 		if err := i.rws.metaWriteSet.add(ns, metaWrite.Key, metadata); err != nil {
	// 			return err
	// 		}
	// 	}
	// }

	return nil
}

func (i *Interceptor) Bytes() ([]byte, error) {
	// operations := map[string]*types.DBOperation{}

	// dbop := types.DBOperation{
	// 	DbName:      i.dbName,
	// 	DataReads:   []*types.DataRead{},
	// 	DataWrites:  []*types.DataWrite{},
	// 	DataDeletes: []*types.DataDelete{},
	// }

	// for ns, keyMap := range i.rws.reads {
	// 	for key, v := range keyMap {
	// 		if v.block != 0 || v.txnum != 0 {
	// 			dbop.DataReads = append(dbop.DataReads, &types.DataRead{
	// 				Key:     key,
	// 				Version: &types.Version{
	// 					BlockNum:             v.block,
	// 					TxNum:                v.txnum,
	// 				},
	// 			})
	// 		} else {
	// 			dbop.DataReads = append(dbop.DataReads, &types.DataRead{
	// 				Key:     key,
	// 				Version: nil,
	// 			})
	// 		}
	// 	}
	// }
	panic("")
	// for ns, keyMap := range i.rws.writes {
	// 	for key, v := range keyMap {
	// rwsb.AddToWriteSet(ns, key, v)
	// }
	// }
	// for ns, keyMap := range i.rws.metawrites {
	// 	for key, v := range keyMap {
	// rwsb.AddToMetadataWriteSet(ns, key, v)
	// }
	// }

	// simRes, err := rwsb.GetTxSimulationResults()
	// if err != nil {
	// 	return nil, err
	// }
	//
	// return simRes.GetPubSimulationBytes()
}

func (i *Interceptor) Equals(other interface{}, nss ...string) error {
	o, ok := other.(*Interceptor)
	if !ok {
		return errors.Errorf("cannot compare to the passed value [%v]", other)
	}

	if err := i.rws.reads.equals(o.rws.reads, nss...); err != nil {
		return errors.Wrap(err, "reads do not match")
	}
	if err := i.rws.writes.equals(o.rws.writes, nss...); err != nil {
		return errors.Wrap(err, "writes do not match")
	}
	if err := i.rws.metawrites.equals(o.rws.metawrites, nss...); err != nil {
		return errors.Wrap(err, "meta writes do not match")
	}

	return nil
}

func (i *Interceptor) String() string {
	return i.rws.String()
}

func (i *Interceptor) Done() {
	logger.Debugf("Done with [%s], closed [%v]", i.txid, i.closed)
	if !i.closed {
		i.closed = true
		i.qe.Done()
	}
}
