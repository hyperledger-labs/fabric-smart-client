/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"testing"

	math "github.com/IBM/mathlib"
	"github.com/stretchr/testify/require"
)

type dummyKVS struct{}

func (d *dummyKVS) Put(key string, value any) error { return nil }
func (d *dummyKVS) Get(key string, value any) error { return nil }

func TestNewBCCSP(t *testing.T) { //nolint:paralleltest

	// Supported curves
	curves := []math.CurveID{
		math.BN254,
		math.BLS12_377_GURVY,
		math.FP256BN_AMCL,
		math.FP256BN_AMCL_MIRACL,
		math.BLS12_381_BBS,
	}

	for _, curveID := range curves {
		bccsp, err := NewBCCSP(curveID)
		require.NoError(t, err)
		require.NotNil(t, bccsp)
	}

	// Unsupported curve
	bccsp, err := NewBCCSP(math.CurveID(3))
	require.ErrorContains(t, err, "unsupported curve ID")
	require.Nil(t, bccsp)
}

func TestNewKSVBCCSP(t *testing.T) { //nolint:paralleltest

	// Using dummyKVS
	kvs := &dummyKVS{}

	// Aries false
	bccsp, err := NewKSVBCCSP(kvs, math.FP256BN_AMCL, false)
	require.NoError(t, err)
	require.NotNil(t, bccsp)

	// Aries true
	bccspAries, err := NewKSVBCCSP(kvs, math.FP256BN_AMCL, true)
	require.NoError(t, err)
	require.NotNil(t, bccspAries)

	// Unsupported curve
	bccspInvalid, err := NewKSVBCCSP(kvs, math.CurveID(3), false)
	require.ErrorContains(t, err, "unsupported curve ID")
	require.Nil(t, bccspInvalid)
}

func TestGetCurveAndTranslator(t *testing.T) { //nolint:paralleltest

	curve, tr, err := GetCurveAndTranslator(math.BN254)
	require.NoError(t, err)
	require.NotNil(t, curve)
	require.NotNil(t, tr)

	curveInvalid, trInvalid, errInvalid := GetCurveAndTranslator(math.CurveID(3))
	require.ErrorContains(t, errInvalid, "unsupported curve ID")
	require.Nil(t, curveInvalid)
	require.Nil(t, trInvalid)
}
