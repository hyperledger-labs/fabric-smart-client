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
	registry3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
)

type myInput struct{}

type myDependentFactory struct {
	*myFactory
}

type myFactory struct{}

func (o *myFactory) NewView([]byte) (view.View, error) { return nil, nil }

func TestInvalidFactory(t *testing.T) {
	t.Parallel()
	c := vfsdk.NewContainer()

	require.Error(t, c.Provide(func() *myInput { return &myInput{} }, vfsdk.WithFactoryId("input")))
}

func TestError(t *testing.T) {
	t.Parallel()
	c := vfsdk.NewContainer()
	require.NoError(t, c.Provide(registry3.NewRegistry))
	require.NoError(t, c.Provide(func() (*myFactory, error) { return nil, errors.New("error occurred") }))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	require.NoError(t, sdk.Install())
	require.NoError(t, sdk.Start(context.Background()))
}

func TestNoReturnError(t *testing.T) {
	t.Parallel()
	c := vfsdk.NewContainer()
	require.NoError(t, c.Provide(registry3.NewRegistry))
	require.NoError(t, c.Provide(func() *myFactory { return &myFactory{} }))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	require.NoError(t, sdk.Install())
	require.NoError(t, sdk.Start(context.Background()))
}

func TestInterdependentFactories(t *testing.T) {
	t.Parallel()
	c := vfsdk.NewContainer()
	require.NoError(t, c.Provide(registry3.NewRegistry))
	require.NoError(t, c.Provide(func(f *myFactory) (*myDependentFactory, error) { return &myDependentFactory{myFactory: f}, nil }, vfsdk.WithFactoryId("a"), vfsdk.WithInitiators("init")))
	require.NoError(t, c.Provide(func() (*myFactory, error) { return &myFactory{}, nil }, vfsdk.WithFactoryId("b")))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	require.NoError(t, sdk.Install())
	require.NoError(t, sdk.Start(context.Background()))
}

func TestRegisterWithoutParams(t *testing.T) {
	t.Parallel()
	c := vfsdk.NewContainer()
	require.NoError(t, c.Provide(registry3.NewRegistry))
	require.NoError(t, c.Provide(func() (*myFactory, error) { return &myFactory{}, nil }, vfsdk.WithFactoryId("abc"), vfsdk.WithInitiators("in1", "in2")))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	require.NoError(t, sdk.Install())
	require.NoError(t, sdk.Start(context.Background()))
}

func TestRegisterWithParams(t *testing.T) {
	t.Parallel()
	c := vfsdk.NewContainer()
	require.NoError(t, c.Provide(registry3.NewRegistry))
	require.NoError(t, c.Provide(func() *myInput { return &myInput{} }))
	require.NoError(t, c.Provide(func(_ *myInput) (*myFactory, error) { return &myFactory{}, nil }, vfsdk.WithFactoryId("abc"), vfsdk.WithInitiators("in1", "in2")))

	sdk := vfsdk.NewFrom(dig.NewBaseSDK(c, nil))
	require.NoError(t, sdk.Install())
	require.NoError(t, sdk.Start(context.Background()))
}
