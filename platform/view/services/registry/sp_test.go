/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package registry

import (
	"reflect"
	"testing"

	"github.com/test-go/testify/assert"
)

type A interface {
	Foo(one string) error
	Boo(one string, two string) error
}

type aImpl struct {
}

func (a *aImpl) Foo(one string) error {
	panic("implement me")
}

func (a *aImpl) Boo(one string, two string) error {
	panic("implement me")
}

type B interface {
	Foo(one string) error
}

func TestSP(t *testing.T) {
	sp := New()
	impl := &aImpl{}
	err := sp.RegisterService(impl)
	assert.NoError(t, err)

	impl2, err := sp.GetService(reflect.TypeOf((*B)(nil)))
	assert.NoError(t, err)
	assert.Equal(t, impl, impl2)

	impl2, err = sp.GetService(reflect.TypeOf((*B)(nil)))
	assert.NoError(t, err)
	assert.Equal(t, impl, impl2)

	impl2, err = sp.GetService((*B)(nil))
	assert.NoError(t, err)
	assert.Equal(t, impl, impl2)

	impl2, err = sp.GetService(&aImpl{})
	assert.NoError(t, err)
	assert.Equal(t, impl, impl2)

	impl2, err = sp.GetService(aImpl{})
	assert.NoError(t, err)
	assert.Equal(t, impl, impl2)

	var b B
	impl2, err = sp.GetService(&b)
	assert.NoError(t, err)
	assert.Equal(t, impl, impl2)
}
