/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeserializer(t *testing.T) {
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)
	id, auditInfo, err := p.Identity(nil)
	require.NoError(t, err)
	eID := p.EnrollmentID()
	ai := &AuditInfo{}
	err = ai.FromBytes(auditInfo)
	require.NoError(t, err)

	require.Equal(t, eID, ai.EnrollmentId)
	require.Equal(t, "auditor.org1.example.com", eID)

	des := &Deserializer{}
	info, err := des.Info(id, nil)
	require.NoError(t, err)
	require.Equal(t, "MSP.x509: [f+hVlmGaPejN2G0XDcESSMX2ol29WPcPQ+Fp3lOARBQ=][apple][auditor.org1.example.com]", info)
}
