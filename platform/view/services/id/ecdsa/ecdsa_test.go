/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ecdsa

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewIdentityFromPEMCertInvalid(t *testing.T) {
	_, _, err := NewIdentityFromPEMCert([]byte("invalid PEM"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot pem decode")
}
