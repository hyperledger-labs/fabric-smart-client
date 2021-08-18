/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type provider struct {
	mutex  sync.Mutex
	relays map[string]*Relay
}

func NewProvider() *provider {
	return &provider{relays: make(map[string]*Relay)}
}

func (w *provider) Relay(fns *fabric.NetworkService) *Relay {
	if fns == nil {
		panic("expected a fabric network service, got nil")
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	r, ok := w.relays[fns.Name()]
	if !ok {
		r = &Relay{
			fns: fns,
		}
		w.relays[fns.Name()] = r
	}
	return r
}

func GetProvider(sp view2.ServiceProvider) *provider {
	s, err := sp.GetService(reflect.TypeOf((*provider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(*provider)
}
