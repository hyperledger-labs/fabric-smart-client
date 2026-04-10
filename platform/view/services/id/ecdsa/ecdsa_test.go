/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ecdsa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIdentityFromPEMCertInvalid(t *testing.T) {
	t.Parallel()
	_, _, err := NewIdentityFromPEMCert([]byte("invalid PEM"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot pem decode")
}
