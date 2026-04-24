/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
)

func TestToPKCS11OptsOpts(t *testing.T) {
	t.Parallel()
	input := &config.PKCS11{
		Security: 256,
		Hash:     "SHA2",
		Library:  "/usr/lib/pkcs11.so",
		Label:    "token1",
		Pin:      "1234",
		KeyIDs: []config.KeyIDMapping{
			{SKI: "abc123", ID: "key1"},
		},
	}
	result := ToPKCS11OptsOpts(input)
	require.Equal(t, 256, result.Security)
	require.Equal(t, "SHA2", result.Hash)
	require.Equal(t, "/usr/lib/pkcs11.so", result.Library)
	require.Len(t, result.KeyIDs, 1)
	require.Equal(t, "abc123", result.KeyIDs[0].SKI)
}

func TestToBCCSPOpts(t *testing.T) {
	t.Parallel()
	input := map[string]interface{}{
		"Default": "SW",
	}
	opts, err := ToBCCSPOpts(input)
	require.NoError(t, err)
	require.Equal(t, "SW", opts.Default)
}

func TestToBCCSPOpts_Invalid(t *testing.T) {
	t.Parallel()
	// Pass something that cannot be decoded
	opts, err := ToBCCSPOpts("not a map")
	require.Error(t, err)
	require.NotNil(t, opts)
}
