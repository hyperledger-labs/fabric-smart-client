/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"testing"

	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"
)

func TestRoleGetValue(t *testing.T) { //nolint:paralleltest
	require.Equal(t, 1, MEMBER.getValue())
	require.Equal(t, 2, ADMIN.getValue())
	require.Equal(t, 4, CLIENT.getValue())
	require.Equal(t, 8, PEER.getValue())
}

func TestCheckRole(t *testing.T) { //nolint:paralleltest
	mask := GetRoleMaskFromIdemixRoles([]Role{ADMIN, PEER})

	require.True(t, CheckRole(mask, ADMIN))
	require.True(t, CheckRole(mask, PEER))
	require.False(t, CheckRole(mask, MEMBER))
	require.False(t, CheckRole(mask, CLIENT))
}

func TestGetRoleMaskFromIdemixRole(t *testing.T) { //nolint:paralleltest
	require.Equal(t, 1, GetRoleMaskFromIdemixRole(MEMBER))
	require.Equal(t, 2, GetRoleMaskFromIdemixRole(ADMIN))
	require.Equal(t, 4, GetRoleMaskFromIdemixRole(CLIENT))
	require.Equal(t, 8, GetRoleMaskFromIdemixRole(PEER))
}

func TestGetIdemixRoleFromMSPRole(t *testing.T) { //nolint:paralleltest
	role := &m.MSPRole{Role: m.MSPRole_ADMIN}
	require.Equal(t, ADMIN.getValue(), GetIdemixRoleFromMSPRole(role))
}

func TestGetIdemixRoleFromMSPRoleType(t *testing.T) { //nolint:paralleltest
	require.Equal(t, ADMIN.getValue(), GetIdemixRoleFromMSPRoleType(m.MSPRole_ADMIN))
	require.Equal(t, CLIENT.getValue(), GetIdemixRoleFromMSPRoleType(m.MSPRole_CLIENT))
	require.Equal(t, MEMBER.getValue(), GetIdemixRoleFromMSPRoleType(m.MSPRole_MEMBER))
	require.Equal(t, PEER.getValue(), GetIdemixRoleFromMSPRoleType(m.MSPRole_PEER))
}

func TestGetIdemixRoleFromMSPRoleValue(t *testing.T) { //nolint:paralleltest
	require.Equal(t, ADMIN.getValue(), GetIdemixRoleFromMSPRoleValue(int(m.MSPRole_ADMIN)))
	require.Equal(t, CLIENT.getValue(), GetIdemixRoleFromMSPRoleValue(int(m.MSPRole_CLIENT)))
	require.Equal(t, MEMBER.getValue(), GetIdemixRoleFromMSPRoleValue(int(m.MSPRole_MEMBER)))
	require.Equal(t, PEER.getValue(), GetIdemixRoleFromMSPRoleValue(int(m.MSPRole_PEER)))
	// Test default case
	require.Equal(t, MEMBER.getValue(), GetIdemixRoleFromMSPRoleValue(999))
}
