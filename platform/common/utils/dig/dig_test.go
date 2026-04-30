/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/dig"
)

type sampleService struct{}

func TestProvideAllReturnsProvideError(t *testing.T) {
	t.Parallel()

	container := dig.New()
	err := ProvideAll(container, 42, "invalid")

	require.Error(t, err)
	require.ErrorContains(t, err, "must provide constructor function")
}

func TestRegisterReturnsRegistrationError(t *testing.T) {
	t.Parallel()

	container := dig.New()
	err := Register[sampleService](container)

	require.Error(t, err)
	require.ErrorContains(t, err, "failed registering type utils.sampleService")
}
