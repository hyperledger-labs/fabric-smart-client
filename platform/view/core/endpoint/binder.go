/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Binder interface {
	GetLongTerm(ephemeral view.Identity) (view.Identity, error)
	Bind(ephemeral, other view.Identity) error
	IsBoundTo(this, that view.Identity) (bool, error)
}

func NewBinder(bindingKVS driver.BindingKVS) *binder {
	return &binder{bindingKVS: bindingKVS}
}

type binder struct {
	bindingKVS driver.BindingKVS
}

func (b *binder) Bind(this, that view.Identity) error {
	if this.IsNone() || that.IsNone() {
		return errors.New("empty ids passed")
	}
	// Make sure the long term is passed, so that the hierarchy is flat and all ephemerals point to the long term
	longTerm, err := b.bindingKVS.GetBinding(that)
	if err != nil {
		return errors.Wrapf(err, "no long term found for [%s]. if long term was passed, it has to be registered first.", that)
	}
	if !longTerm.IsNone() {
		logger.Debugf("Long term id for [%s] is [%s]", that, longTerm)
		return b.bindingKVS.PutBinding(this, longTerm)
	}

	logger.Debugf("Id [%s] has no long term binding. It will be registered as a long-term id.", that)
	if err := b.bindingKVS.PutBinding(that, that); err != nil {
		return errors.Wrapf(err, "failed to register [%s] as long term", that)
	}
	err = b.bindingKVS.PutBinding(this, that)
	if lt, err := b.bindingKVS.GetBinding(this); err != nil || lt.IsNone() {
		logger.Errorf("wrong binding for [%s][%s]: %v", that, lt, err)
	} else {
		logger.Errorf("successful binding for [%s]", that)
	}
	return err
}

func (b *binder) RegisterLongTerm(longTerm view.Identity) error {
	// Self reference that indicates that a binding is long term
	return b.bindingKVS.PutBinding(longTerm, longTerm)
}

func (b *binder) GetLongTerm(ephemeral view.Identity) (view.Identity, error) {
	return b.bindingKVS.GetBinding(ephemeral)
}

func (b *binder) IsBoundTo(this, that view.Identity) (bool, error) {
	return b.bindingKVS.HaveSameBinding(this, that)
}
