/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vfsdk_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/vfsdk"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	registry3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type myInput struct{}

type myDependentFactory struct {
	*myFactory
}

type myFactory struct{}

func (o *myFactory) NewView([]byte) (view.View, error) { return nil, nil }

func TestInvalidFactory(t *testing.T) {
	c := vfsdk.NewContainer()

	assert.Error(c.Provide(func() *myInput { return &myInput{} }, vfsdk.WithFactoryId("input")))
}

func TestError(t *testing.T) {
	c := vfsdk.NewContainer()
	assert.NoError(c.Provide(registry3.NewRegistry))
	assert.NoError(c.Provide(func() (*myFactory, error) { return nil, errors.New("error occurred") }))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	assert.NoError(sdk.Install())
	assert.NoError(sdk.Start(context.Background()))
}

func TestNoReturnError(t *testing.T) {
	c := vfsdk.NewContainer()
	assert.NoError(c.Provide(registry3.NewRegistry))
	assert.NoError(c.Provide(func() *myFactory { return &myFactory{} }))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	assert.NoError(sdk.Install())
	assert.NoError(sdk.Start(context.Background()))
}

func TestInterdependentFactories(t *testing.T) {
	c := vfsdk.NewContainer()
	assert.NoError(c.Provide(registry3.NewRegistry))
	assert.NoError(c.Provide(func(f *myFactory) (*myDependentFactory, error) { return &myDependentFactory{myFactory: f}, nil }, vfsdk.WithFactoryId("a"), vfsdk.WithInitiators("init")))
	assert.NoError(c.Provide(func() (*myFactory, error) { return &myFactory{}, nil }, vfsdk.WithFactoryId("b")))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	assert.NoError(sdk.Install())
	assert.NoError(sdk.Start(context.Background()))
}

func TestRegisterWithoutParams(t *testing.T) {
	c := vfsdk.NewContainer()
	assert.NoError(c.Provide(registry3.NewRegistry))
	assert.NoError(c.Provide(func() (*myFactory, error) { return &myFactory{}, nil }, vfsdk.WithFactoryId("abc"), vfsdk.WithInitiators("in1", "in2")))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	assert.NoError(sdk.Install())
	assert.NoError(sdk.Start(context.Background()))
}

func TestRegisterWithParams(t *testing.T) {
	c := vfsdk.NewContainer()
	assert.NoError(c.Provide(registry3.NewRegistry))
	assert.NoError(c.Provide(func() *myInput { return &myInput{} }))
	assert.NoError(c.Provide(func(_ *myInput) (*myFactory, error) { return &myFactory{}, nil }, vfsdk.WithFactoryId("abc"), vfsdk.WithInitiators("in1", "in2")))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	assert.NoError(sdk.Install())
	assert.NoError(sdk.Start(context.Background()))
}
