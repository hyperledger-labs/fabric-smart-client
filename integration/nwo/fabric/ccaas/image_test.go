/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureImagePresentMissing(t *testing.T) { //nolint:paralleltest
	err := ensureImagePresent("fsc-cc/base:latest",
		func(string) (bool, error) { return false, nil })
	require.Error(t, err)
	require.Contains(t, err.Error(), "fsc-cc/base:latest")
	require.Contains(t, err.Error(), "make chaincode-images")
}

func TestEnsureImagePresentFound(t *testing.T) { //nolint:paralleltest
	require.NoError(t, ensureImagePresent("fsc-cc/base:latest",
		func(string) (bool, error) { return true, nil }))
}
