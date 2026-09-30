/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func TestNewInvalidOpts(t *testing.T) { //nolint:paralleltest
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	i, err := New(nil, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid msp.NewOpts instance. It must be either *BCCSPNewOpts or *IdemixNewOpts. It was [<nil>]")
	require.Nil(t, i)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: -1}}, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid *BCCSPNewOpts. Version not recognized [-1]")
	require.Nil(t, i)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: -1}}, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [-1]")
	require.Nil(t, i)
}

func TestNew(t *testing.T) { //nolint:paralleltest
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	i, err := New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_0}}, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, i)
	require.Equal(t, MSPVersion(MSPv1_0), i.(*bccspmsp).version)
	require.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).setupV1).Pointer()).Name(),
	)
	require.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).validateIdentityOUsV1).Pointer()).Name(),
	)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_1}}, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, i)
	require.Equal(t, MSPVersion(MSPv1_1), i.(*bccspmsp).version)
	require.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).setupV11).Pointer()).Name(),
	)
	require.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).validateIdentityOUsV11).Pointer()).Name(),
	)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_0}}, cryptoProvider)
	require.Error(t, err)
	require.Nil(t, i)
	require.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [0]")

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_1}}, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, i)
}

func TestNewBCCSPVersions(t *testing.T) { //nolint:paralleltest
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	funcName := func(f any) string { return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name() }

	for _, tc := range []struct {
		version  MSPVersion
		expected func(*bccspmsp) []any
	}{
		{MSPv1_3, func(b *bccspmsp) []any {
			return []any{b.setupV11, b.validateIdentityOUsV11, b.satisfiesPrincipalInternalV13}
		}},
		{MSPv1_4_3, func(b *bccspmsp) []any {
			return []any{b.setupV142, b.validateIdentityOUsV142, b.satisfiesPrincipalInternalV142}
		}},
		{MSPv3_0, func(b *bccspmsp) []any {
			return []any{b.setupV3, b.validateIdentityOUsV142, b.satisfiesPrincipalInternalV142}
		}},
	} {
		i, err := New(&BCCSPNewOpts{NewBaseOpts{Version: tc.version}}, cryptoProvider)
		require.NoError(t, err)
		b := i.(*bccspmsp)
		require.Equal(t, tc.version, b.GetVersion())
		expected := tc.expected(b)
		require.Equal(t, funcName(expected[0]), funcName(b.internalSetupFunc))
		require.Equal(t, funcName(expected[1]), funcName(b.internalValidateIdentityOusFunc))
		require.Equal(t, funcName(expected[2]), funcName(b.internalSatisfiesPrincipalInternalFunc))
	}
}

func TestNewBccspMspInvalidVersion(t *testing.T) { //nolint:paralleltest
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	_, err = newBccspMsp(-1, cryptoProvider)
	require.EqualError(t, err, "Invalid MSP version [-1]")

	_, err = NewBccspMspWithKeyStore(-1, sw.NewDummyKeyStore(), cryptoProvider)
	require.EqualError(t, err, "Invalid MSP version [-1]")
}
