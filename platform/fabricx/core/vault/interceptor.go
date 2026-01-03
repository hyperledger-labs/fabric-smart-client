/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-x-committer/api/types"
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
	logger.Infof("[Interceptor.Bytes] START txID=%s, namespaces=%v, isClosed=%v", i.txID, i.Namespaces(), i.IsClosed())
	if i.IsClosed() {
		logger.Warnf("interceptor already closed!")
		// TODO: we need to handle this case better; currently it only works when bytes was called before
		cached := i.marshallingCache.Load()
		if cached != nil {
			logger.Infof("[Interceptor.Bytes] Returning cached bytes (len=%d)", len(*cached))
		} else {
			logger.Warnf("[Interceptor.Bytes] Cache is nil!")
		}
		return *cached, nil
	}

	nsInfo, err := namespaceVersions(i.qe, i.Namespaces()...)
	if err != nil {
		logger.Errorf("[Interceptor.Bytes] Error getting namespace versions: %v", err)
		return nil, err
	}

	logger.Infof("[Interceptor.Bytes] >> nsInfo: %v", nsInfo)
	for ns, ver := range nsInfo {
		logger.Infof("[Interceptor.Bytes] Namespace=%s, Version=%x", ns, ver)
	}
	b, err := i.m.marshal(i.txID, i.RWs(), nsInfo)
	if err != nil {
		logger.Errorf("[Interceptor.Bytes] Marshal error: %v", err)
		return nil, err
	}

	logger.Infof("[Interceptor.Bytes] Marshalled bytes (len=%d), first 64 bytes: %x", len(b), b[:min(64, len(b))])
	i.marshallingCache.Store(&b)
	return b, nil
}

func (i *Interceptor[V]) AppendRWSet(raw []byte, nss ...string) error {
	if i.IsClosed() {
		return errors.New("this instance was closed")
	}

	return i.m.Append(i.RWs(), raw, nss...)
}

func namespaceVersions(qe vault.VersionedQueryExecutor, namespaces ...string) (map[string][]byte, error) {
	logger.Infof("[namespaceVersions] Getting versions for namespaces: %v", namespaces)
	nsInfo := make(map[string][]byte)

	var errs error
	for _, ns := range namespaces {
		logger.Debugf("[namespaceVersions] Reading state for MetaNamespaceID=%s, ns=%s", types.MetaNamespaceID, ns)
		v, err := qe.GetState(context.TODO(), types.MetaNamespaceID, ns)
		if err != nil {
			logger.Errorf("Ouch! error when reading %v-%v: %v", types.MetaNamespaceID, ns, err)
			errs = errors.Join(errs, err)
		}

		if v == nil {
			logger.Infof("[namespaceVersions] %v-%v does not exist, using version 0", types.MetaNamespaceID, ns)
		} else {
			logger.Infof("[namespaceVersions] %v-%v exists, version=%x, raw=%v", types.MetaNamespaceID, ns, v.Version, v.Raw)
		}

		nsVersion := Marshal(0)
		if v != nil {
			nsVersion = v.Version
		}
		logger.Infof("[namespaceVersions] Final version for ns=%s: %x", ns, nsVersion)

		nsInfo[ns] = nsVersion
	}
	if errs != nil {
		return nil, errs
	}

	return nsInfo, nil
}
