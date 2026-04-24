/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditInfo_RoundTrip(t *testing.T) {
	t.Parallel()
	ai := &AuditInfo{
		EnrollmentId:     "user1",
		RevocationHandle: []byte("handle123"),
	}
	raw, err := ai.Bytes()
	require.NoError(t, err)

	ai2 := &AuditInfo{}
	require.NoError(t, ai2.FromBytes(raw))
	require.Equal(t, ai.EnrollmentId, ai2.EnrollmentId)
	require.Equal(t, ai.RevocationHandle, ai2.RevocationHandle)
}

func TestAuditInfo_FromBytes_Invalid(t *testing.T) {
	t.Parallel()
	ai := &AuditInfo{}
	require.Error(t, ai.FromBytes([]byte("not json")))
}
