/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"sync/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
)

type Interceptor[V driver.ValidationCode] struct {
	*vault.Interceptor[V]

	txID driver.TxID
	qe   vault.VersionedQueryExecutor
	m    *Marshaller

	// caches the serialized rwset
	marshallingCache atomic.Pointer[[]byte]
}

func newInterceptor[V driver.ValidationCode](in *vault.Interceptor[V], txID driver.TxID, qe vault.VersionedQueryExecutor, m *Marshaller) *Interceptor[V] {
	return &Interceptor[V]{
		Interceptor: in,
		txID:        txID,
		qe:          qe,
		m:           m,
	}
}

func (i *Interceptor[V]) Bytes() ([]byte, error) {
	// if already closed, return cached bytes if available.
	if i.IsClosed() {
		logger.Warnf("interceptor already closed!")
		return i.cachedBytesOrError()
	}

	nsInfo, err := namespaceVersions(i.qe, i.Namespaces()...)
	if err != nil {
		// The interceptor may have been closed concurrently between the
		// IsClosed() check above and the qe access in namespaceVersions.
		// If so, fall back to the cache instead of propagating the error.
		if i.IsClosed() {
			if cached, cacheErr := i.cachedBytesOrError(); cacheErr == nil {
				return cached, nil
			}
		}
		return nil, err
	}

	logger.Debugf(">> nsInfo: %v", nsInfo)
	b, err := i.m.marshal(i.txID, i.RWs(), nsInfo)
	if err != nil {
		return nil, err
	}

	i.marshallingCache.Store(&b)
	return b, nil
}

// cachedBytesOrError returns previously cached marshalled bytes, or an error
// if Bytes() was never successfully called before the interceptor was closed.
func (i *Interceptor[V]) cachedBytesOrError() ([]byte, error) {
	cached := i.marshallingCache.Load()
	if cached == nil {
		return nil, errors.Errorf("interceptor for tx [%s] was closed before marshalling was cached", i.txID)
	}
	return *cached, nil
}

func (i *Interceptor[V]) AppendRWSet(raw []byte, nss ...string) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	return i.m.Append(i.RWs(), raw, nss...)
}

func namespaceVersions(qe vault.VersionedQueryExecutor, namespaces ...string) (map[string][]byte, error) {
	nsInfo := make(map[string][]byte)

	var errs error
	for _, ns := range namespaces {
		v, err := qe.GetState(context.TODO(), committerpb.MetaNamespaceID, ns)
		if err != nil {
			logger.Errorf("Ouch! error when reading %v-%v: %v", committerpb.MetaNamespaceID, ns, err)
			errs = errors.Join(errs, err)
		}

		if v == nil {
			logger.Debugf("Ouch! %v-%v does not exist", committerpb.MetaNamespaceID, ns)
		}

		nsVersion := Marshal(0)
		if v != nil {
			nsVersion = v.Version
		}

		nsInfo[ns] = nsVersion
	}
	if errs != nil {
		return nil, errs
	}

	return nsInfo, nil
}
