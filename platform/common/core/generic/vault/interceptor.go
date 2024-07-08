/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/pkg/errors"
)

type VersionedQueryExecutor interface {
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	GetState(namespace, key string) (VersionedValue, error)
	Done()
}

type Interceptor[V driver.ValidationCode] struct {
	Logger     Logger
	QE         VersionedQueryExecutor
	TxIDStore  TXIDStoreReader[V]
	Rws        ReadWriteSet
	Marshaller Marshaller
	Closed     bool
	TxID       string
	vcProvider driver.ValidationCodeProvider[V] // TODO
	Mutex      sync.RWMutex
}

func EmptyRWSet() ReadWriteSet {
	return ReadWriteSet{
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
	}
}

func NewInterceptor[V driver.ValidationCode](
	logger Logger,
	qe VersionedQueryExecutor,
	txIDStore TXIDStoreReader[V],
	txID driver.TxID,
	vcProvider driver.ValidationCodeProvider[V],
	marshaller Marshaller,
) *Interceptor[V] {
	logger.Debugf("new interceptor [%s]", txID)

	return &Interceptor[V]{
		Logger:     logger,
		TxID:       txID,
		QE:         qe,
		TxIDStore:  txIDStore,
		Rws:        EmptyRWSet(),
		vcProvider: vcProvider,
		Marshaller: marshaller,
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
				vv, err := i.QE.GetState(ns, k)
				if err != nil {
					return err
				}

				if vv.Block != v.Block || vv.TxNum != v.TxNum {
					return errors.Errorf("invalid read: vault at version %s:%s %d:%d, read-write set at version %d:%d", ns, k, vv.Block, vv.TxNum, v.Block, v.TxNum)
				}
			}
		}
	}
	return nil
}

func (i *Interceptor[V]) Clear(ns string) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	i.Rws.ReadSet.Clear(ns)
	i.Rws.WriteSet.Clear(ns)
	i.Rws.MetaWriteSet.Clear(ns)

	return nil
}

func (i *Interceptor[V]) GetReadKeyAt(ns string, pos int) (string, error) {
	if i.IsClosed() {
		return "", errors.New("this instance was closed")
	}

	key, in := i.Rws.ReadSet.GetAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	return key, nil
}

func (i *Interceptor[V]) GetReadAt(ns string, pos int) (string, []byte, error) {
	if i.IsClosed() {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.Rws.ReadSet.GetAt(ns, pos)
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
	if i.IsClosed() {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.Rws.WriteSet.GetAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.Rws.WriteSet.Get(ns, key), nil
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
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	return i.SetState(namespace, key, nil)
}

func (i *Interceptor[V]) SetState(namespace string, key string, value []byte) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}
	i.Logger.Debugf("SetState [%s,%s,%s]", namespace, key, hash.Hashable(value).String())

	return i.Rws.WriteSet.Add(namespace, key, value)
}

func (i *Interceptor[V]) SetStateMetadata(namespace string, key string, value map[string][]byte) error {
	if i.Closed {
		return errors.New("this instance was closed")
	}

	return i.Rws.MetaWriteSet.Add(namespace, key, value)
}

func (i *Interceptor[V]) GetStateMetadata(namespace, key string, opts ...driver.GetStateOpt) (map[string][]byte, error) {
	if i.IsClosed() {
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

		b, t, in := i.Rws.ReadSet.Get(namespace, key)
		if in {
			if b != block || t != txnum {
				return nil, errors.Errorf("invalid metadata read: previous value returned at version %d:%d, current value at version %d:%d", b, t, block, txnum)
			}
		} else {
			i.Rws.ReadSet.Add(namespace, key, block, txnum)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.Rws.MetaWriteSet.Get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetStateMetadata(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.Rws.WriteSet.In(namespace, key) {
			return val, err
		}

		return i.GetStateMetadata(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor[V]) GetState(namespace string, key string, opts ...driver.GetStateOpt) ([]byte, error) {
	if i.IsClosed() {
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
		vv, err := i.QE.GetState(namespace, key)
		if err != nil {
			return nil, err
		}

		b, t, in := i.Rws.ReadSet.Get(namespace, key)
		if in {
			if b != vv.Block || t != vv.TxNum {
				return nil, errors.Errorf("invalid read [%s:%s]: previous value returned at version %d:%d, current value at version %d:%d", namespace, key, b, t, vv.Block, vv.TxNum)
			}
		} else {
			i.Rws.ReadSet.Add(namespace, key, vv.Block, vv.TxNum)
		}

		return vv.Raw, nil

	case driver.FromIntermediate:
		return i.Rws.WriteSet.Get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetState(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.Rws.WriteSet.In(namespace, key) {
			return val, err
		}

		return i.GetState(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor[V]) AppendRWSet(raw []byte, nss ...string) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	return i.Marshaller.Append(&i.Rws, raw, nss...)
}

func (i *Interceptor[V]) Bytes() ([]byte, error) {
	return i.Marshaller.Marshal(&i.Rws)
}

func (i *Interceptor[V]) Equals(other interface{}, nss ...string) error {
	switch o := other.(type) {
	case *Interceptor[V]:
		if err := i.Rws.Reads.Equals(o.Rws.Reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.Rws.Writes.Equals(o.Rws.Writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.Rws.MetaWrites.Equals(o.Rws.MetaWrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	case *Inspector:
		if err := i.Rws.Reads.Equals(o.Rws.Reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.Rws.Writes.Equals(o.Rws.Writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.Rws.MetaWrites.Equals(o.Rws.MetaWrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	default:
		return errors.Errorf("cannot compare to the passed value [%v]", other)
	}
	return nil
}

func (i *Interceptor[V]) Done() {
	i.Logger.Debugf("Done with [%s], closed [%v]", i.TxID, i.Closed)
	if !i.IsClosed() {
		i.Closed = true
		if i.QE != nil {
			i.QE.Done()
		}
	}
}

func (i *Interceptor[V]) Reopen(qe VersionedQueryExecutor) error {
	i.Logger.Debugf("Reopen with [%s], closed [%v]", i.TxID, i.Closed)
	if !i.Closed {
		return errors.Errorf("already open")
	}
	i.QE = qe
	i.Closed = false

	return nil
}

func (i *Interceptor[V]) IsClosed() bool {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	return i.Closed
}

func (i *Interceptor[V]) RWs() *ReadWriteSet {
	return &i.Rws
}
