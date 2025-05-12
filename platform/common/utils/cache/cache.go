/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"

var logger = logging.MustGetLogger()

type Map[K comparable, V any] interface {
	Get(K) (V, bool)
	Put(K, V)
	Update(K, func(bool, V) (bool, V)) bool
	Delete(...K)
	Len() int
}

type rwLock interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

type noLock struct{}

func (l *noLock) Lock()    {}
func (l *noLock) Unlock()  {}
func (l *noLock) RLock()   {}
func (l *noLock) RUnlock() {}
