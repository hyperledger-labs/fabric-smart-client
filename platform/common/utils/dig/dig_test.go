/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/mock"
)

type sampleService struct {
	Name string
}

func TestVisualize(t *testing.T) {
	t.Parallel()

	container := dig.New()
	require.NoError(t, container.Provide(func() string { return "hello" }))

	visualization := Visualize(container)
	require.Contains(t, visualization, "digraph")
}

func TestProvideAll(t *testing.T) {
	t.Parallel()

	container := dig.New()
	err := ProvideAll(
		container,
		func() string { return "hello" },
		func(s string) int { return len(s) },
	)
	require.NoError(t, err)

	err = container.Invoke(func(length int) {
		require.Equal(t, 5, length)
	})
	require.NoError(t, err)
}

func TestProvideAllReturnsJoinedErrors(t *testing.T) {
	t.Parallel()

	container := dig.New()
	err := ProvideAll(container, 42, "invalid")
	require.Error(t, err)
	require.ErrorContains(t, err, "must provide constructor function")
}

func TestRegister(t *testing.T) {
	t.Parallel()

	registry := &mock.ServiceRegistry{}
	container := dig.New()
	require.NoError(t, container.Provide(func() services.Registry {
		return registry
	}))
	require.NoError(t, container.Provide(func() sampleService {
		return sampleService{Name: "alpha"}
	}))

	require.NoError(t, Register[sampleService](container))
	require.Equal(t, 1, registry.RegisterServiceCallCount())
	require.Equal(t, sampleService{Name: "alpha"}, registry.RegisterServiceArgsForCall(0))
}

func TestRegisterReturnsHelpfulError(t *testing.T) {
	t.Parallel()

	container := dig.New()
	err := Register[sampleService](container)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed registering type utils.sampleService")
}

func TestIdentity(t *testing.T) {
	t.Parallel()

	fn := Identity[int]()
	require.Equal(t, 42, fn(42))
}
