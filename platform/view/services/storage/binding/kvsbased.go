/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binding

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

func NewKVSBased(kvss *kvs.KVS) *kvsBased {
	return &kvsBased{e: kvs.NewEnhancedKVS[view.Identity, view.Identity](kvss, bindingKey)}
}

type kvsBased struct {
	e *kvs.EnhancedKVS[view.Identity, view.Identity]
}

func (kvs *kvsBased) HaveSameBinding(this, that view.Identity) (bool, error) {
	thisBinding, err := kvs.e.Get(this)
	if err != nil {
		return false, err
	}
	thatBinding, err := kvs.e.Get(that)
	if err != nil {
		return false, err
	}
	return thisBinding.Equal(thatBinding), nil
}
func (kvs *kvsBased) GetLongTerm(ephemeral view.Identity) (view.Identity, error) {
	return kvs.e.Get(ephemeral)
}
func (kvs *kvsBased) PutBinding(ephemeral, longTerm view.Identity) error {
	if ephemeral.IsNone() || longTerm.IsNone() {
		return errors.New("empty ids passed")
	}
	// Make sure the long term is passed, so that the hierarchy is flat and all ephemerals point to the long term
	longTerm, err := kvs.e.Get(longTerm)
	if err != nil {
		return errors.Wrapf(err, "no long term found for [%s]. if long term was passed, it has to be registered first.", longTerm)
	}
	if longTerm != nil {
		return kvs.e.Put(ephemeral, longTerm)
	}

	if err := kvs.e.Put(longTerm, longTerm); err != nil {
		return errors.Wrapf(err, "failed to register [%s] as long term", longTerm)
	}
	return kvs.e.Put(ephemeral, longTerm)
}

func bindingKey(ephemeral view.Identity) (string, error) {
	return kvs.CreateCompositeKey("platform.fsc.endpoint.binding", []string{ephemeral.UniqueID()})
}
