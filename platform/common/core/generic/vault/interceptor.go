/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
)

type VersionedQueryExecutor interface {
	GetStateMetadata(ctx context.Context, namespace, key string) (driver.Metadata, driver.RawVersion, error)
	GetState(ctx context.Context, namespace, key string) (*driver.VaultRead, error)
	Done() error
}

type VersionComparator interface {
	Equal(v1, v2 driver.RawVersion) bool
}

type Interceptor[V driver.ValidationCode] struct {
	logger            Logger
	qe                VersionedQueryExecutor
	vaultStore        TxStatusStore
	rws               ReadWriteSet
	marshaller        Marshaller
	versionComparator VersionComparator
	closed            bool
	txID              driver.TxID
	ctx               context.Context
	vcProvider        driver.ValidationCodeProvider[V] // TODO
	sync.RWMutex
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
	ctx context.Context,
	rwSet ReadWriteSet,
	qe VersionedQueryExecutor,
	vaultStore TxStatusStore,
	txID driver.TxID,
	vcProvider driver.ValidationCodeProvider[V],
	marshaller Marshaller,
	versionComparator VersionComparator,
) *Interceptor[V] {
	logger.Debugf("new interceptor [%s]", txID)

	return &Interceptor[V]{
		logger:            logger,
		ctx:               ctx,
		txID:              txID,
		qe:                qe,
		vaultStore:        vaultStore,
		rws:               rwSet,
		vcProvider:        vcProvider,
		marshaller:        marshaller,
		versionComparator: versionComparator,
	}
}

func (i *Interceptor[V]) IsValid() error {
	i.RLock()
	defer i.RUnlock()
	if i.qe == nil {
		return nil
	}
	for ns, nsMap := range i.rws.Reads {
		for k, v := range nsMap {
			vv, err := i.qe.GetState(context.Background(), ns, k)
			if err != nil {
				return err
			}
			if !i.versionComparator.Equal(v, vv.Version) {
				return errors.Errorf("invalid read: vault at version %s:%s [%v], read-write set at version [%v]", ns, k, vv, v)
			}
		}
	}
	return nil
}

func (i *Interceptor[V]) Clear(ns string) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	i.rws.ReadSet.Clear(ns)
	i.rws.WriteSet.Clear(ns)
	i.rws.MetaWriteSet.Clear(ns)

	return nil
}

func (i *Interceptor[V]) AddReadAt(ns driver.Namespace, key string, version Version) error {
	i.rws.ReadSet.Add(ns, key, version)
	return nil
}

func (i *Interceptor[V]) GetReadKeyAt(ns string, pos int) (string, error) {
	if i.IsClosed() {
		return "", errors.New("this instance was closed")
	}

	key, in := i.rws.ReadSet.GetAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	return key, nil
}

func (i *Interceptor[V]) GetReadAt(ns string, pos int) (string, []byte, error) {
	if i.IsClosed() {
		return "", nil, errors.New("this instance was closed")
	}

	key, in := i.rws.ReadSet.GetAt(ns, pos)
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

	key, in := i.rws.WriteSet.GetAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.rws.WriteSet.Get(ns, key), nil
}

func (i *Interceptor[V]) NumReads(ns string) int {
	return len(i.rws.Reads[ns])
}

func (i *Interceptor[V]) NumWrites(ns string) int {
	return len(i.rws.Writes[ns])
}

func (i *Interceptor[V]) Namespaces() []string {
	mergedMaps := map[string]struct{}{}

	for ns := range i.rws.Reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.rws.Writes {
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
	i.logger.Debugf("SetState [%s,%s,%s]", namespace, key, hash.Hashable(value).String())

	return i.rws.WriteSet.Add(namespace, key, value)
}

func (i *Interceptor[V]) SetStateMetadata(namespace string, key string, value map[string][]byte) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	return i.rws.MetaWriteSet.Add(namespace, key, value)
}

func (i *Interceptor[V]) SetStateMetadatas(ns driver.Namespace, kvs map[driver.PKey]driver.Metadata) map[driver.PKey]error {
	errs := make(map[driver.PKey]error)
	for pkey, value := range kvs {
		if err := i.SetStateMetadata(ns, pkey, value); err != nil {
			errs[pkey] = err
		}
	}
	return errs
}

func (i *Interceptor[V]) GetStateMetadata(namespace, key string, opts ...driver.GetStateOpt) (map[string][]byte, error) {
	if i.IsClosed() {
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
		i.RLock()
		if i.qe == nil {
			i.RUnlock()
			return nil, errors.New("this instance is write only")
		}
		val, vaultVersion, err := i.qe.GetStateMetadata(i.ctx, namespace, key)
		if err != nil {
			i.RUnlock()
			return nil, err
		}
		i.RUnlock()

		version, in := i.rws.ReadSet.Get(namespace, key)
		i.logger.Debugf("got states: [%v] - [%v]", version, vaultVersion)
		if in {
			if !i.versionComparator.Equal(version, vaultVersion) {
				return nil, errors.Errorf("invalid metadata read: previous value returned at version [%v], current value at version [%v]", version, vaultVersion)
			}
		} else {
			i.rws.ReadSet.Add(namespace, key, vaultVersion)
		}

		return val, nil

	case driver.FromIntermediate:
		return i.rws.MetaWriteSet.Get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetStateMetadata(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.rws.WriteSet.In(namespace, key) {
			return val, err
		}

		return i.GetStateMetadata(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor[V]) GetState(namespace driver.Namespace, key driver.PKey, opts ...driver.GetStateOpt) ([]byte, error) {
	if i.IsClosed() {
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
		i.RLock()
		if i.qe == nil {
			i.RUnlock()
			return nil, errors.New("this instance is write only")
		}
		vv, err := i.qe.GetState(i.ctx, namespace, key)
		if err != nil {
			i.RUnlock()
			return nil, err
		}
		i.RUnlock()
		var vaultVersion Version
		if vv != nil {
			vaultVersion = vv.Version
		}
		version, in := i.rws.ReadSet.Get(namespace, key)
		if in {
			if !i.versionComparator.Equal(version, vaultVersion) {
				return nil, errors.Errorf("invalid read [%s:%s]: previous value returned at version [%v], current value at version [%v]", namespace, key, version, vaultVersion)
			}
		} else {
			i.rws.ReadSet.Add(namespace, key, vaultVersion)
		}
		if vv == nil {
			return []byte(nil), nil
		}
		return vv.Raw, nil

	case driver.FromIntermediate:
		return i.rws.WriteSet.Get(namespace, key), nil

	case driver.FromBoth:
		val, err := i.GetState(namespace, key, driver.FromIntermediate)
		if err != nil || val != nil || i.rws.WriteSet.In(namespace, key) {
			return val, err
		}

		return i.GetState(namespace, key, driver.FromStorage)

	default:
		return nil, errors.Errorf("invalid get option %+v", opts)
	}
}

func (i *Interceptor[V]) GetDirectState(namespace driver.Namespace, key string) ([]byte, error) {
	vv, err := i.qe.GetState(i.ctx, namespace, key)
	if err != nil {
		return nil, err
	}
	return vv.Raw, nil
}

func (i *Interceptor[V]) AppendRWSet(raw []byte, nss ...string) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	return i.marshaller.Append(&i.rws, raw, nss...)
}

func (i *Interceptor[V]) Bytes() ([]byte, error) {
	return i.marshaller.Marshal(i.txID, &i.rws)
}

func (i *Interceptor[V]) Equals(other interface{}, nss ...string) error {
	switch o := other.(type) {
	case *Interceptor[V]:
		if err := i.rws.Reads.Equals(o.rws.Reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.rws.Writes.Equals(o.rws.Writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.rws.MetaWrites.Equals(o.rws.MetaWrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	case *Inspector:
		if err := i.rws.Reads.Equals(o.Rws.Reads, nss...); err != nil {
			return errors.Wrap(err, "reads do not match")
		}
		if err := i.rws.Writes.Equals(o.Rws.Writes, nss...); err != nil {
			return errors.Wrap(err, "writes do not match")
		}
		if err := i.rws.MetaWrites.Equals(o.Rws.MetaWrites, nss...); err != nil {
			return errors.Wrap(err, "meta writes do not match")
		}
	default:
		return errors.Errorf("cannot compare to the passed value [%v]", other)
	}
	return nil
}

func (i *Interceptor[V]) Done() {
	i.logger.Debugf("Done with [%s], closed [%v]", i.txID, i.IsClosed())
	i.Lock()
	defer i.Unlock()
	if !i.closed {
		i.closed = true
		if i.qe != nil {
			if err := i.qe.Done(); err != nil {
				i.logger.Warnf("Failed to close qe: [%v]", err)
			}
		}
	}
}

func (i *Interceptor[V]) Reopen(qe VersionedQueryExecutor) error {
	i.logger.Debugf("Reopen with [%s], closed [%v]", i.txID, i.IsClosed())
	if !i.IsClosed() {
		return errors.Errorf("already open")
	}
	i.Lock()
	defer i.Unlock()
	i.qe = qe
	i.closed = false
	return nil
}

func (i *Interceptor[V]) IsClosed() bool {
	i.RLock()
	defer i.RUnlock()
	return i.closed
}

func (i *Interceptor[V]) RWs() *ReadWriteSet {
	return &i.rws
}
