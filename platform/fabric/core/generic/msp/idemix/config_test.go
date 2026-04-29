/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"testing"

	"github.com/IBM/idemix/idemixmsp"
	math "github.com/IBM/mathlib"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
)

func TestGetFabricCAIdemixMspConfigPreservesCurveID(t *testing.T) {
	t.Parallel()

	conf, err := idemix2.GetFabricCAIdemixMspConfig("./testdata/charlie.ExtraId2", "charlie.ExtraId2")
	require.NoError(t, err)

	var idemixConf idemixmsp.IdemixMSPConfig
	err = proto.UnmarshalV1(conf.Config, &idemixConf)
	require.NoError(t, err)

	require.Equal(t, "gurvy.Bn254", idemixConf.CurveId)
	require.NotNil(t, idemixConf.Signer)
	require.Equal(t, "gurvy.Bn254", idemixConf.Signer.CurveId)
}

func TestCurveIDFromString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		input    string
		expected math.CurveID
	}{
		{name: "default empty", input: "", expected: math.FP256BN_AMCL},
		{name: "trimmed amcl", input: "  amcl.Fp256bn ", expected: math.FP256BN_AMCL},
		{name: "bn254 short alias", input: "bn254", expected: math.BN254},
		{name: "bn254 gurvy alias", input: "gurvy.Bn254", expected: math.BN254},
		{name: "miracl alias", input: "fp256bn_amcl_miracl", expected: math.FP256BN_AMCL_MIRACL},
		{name: "bls12_381", input: "bls12_381", expected: math.BLS12_381},
		{name: "bls12_377 gurvy alias", input: "gurvy.bls12_377", expected: math.BLS12_377_GURVY},
		{name: "bls12_381 gurvy alias", input: "bls12_381_gurvy", expected: math.BLS12_381_GURVY},
		{name: "bls12_381 bbs", input: "bls12_381_bbs", expected: math.BLS12_381_BBS},
		{name: "bls12_381 bbs gurvy", input: "bls12_381_bbs_gurvy", expected: math.BLS12_381_BBS_GURVY},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			curveID, err := idemix2.CurveIDFromString(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, curveID)
		})
	}
}

func TestCurveIDFromStringInvalid(t *testing.T) {
	t.Parallel()

	_, err := idemix2.CurveIDFromString("not-a-curve")
	require.EqualError(t, err, "unsupported curve id [not-a-curve]")
}

func TestCurveIDFromMSPConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		curveID, err := idemix2.CurveIDFromMSPConfig(nil)
		require.Equal(t, math.FP256BN_AMCL, curveID)
		require.EqualError(t, err, "setup error: nil conf reference")
	})

	t.Run("invalid protobuf", func(t *testing.T) {
		t.Parallel()

		curveID, err := idemix2.CurveIDFromMSPConfig(&m.MSPConfig{Config: []byte("garbage")})
		require.Equal(t, math.FP256BN_AMCL, curveID)
		require.ErrorContains(t, err, "failed unmarshalling idemix provider config")
	})

	t.Run("top-level curve id", func(t *testing.T) {
		t.Parallel()

		conf := &idemixmsp.IdemixMSPConfig{CurveId: "BLS12_381"}
		raw, err := proto.MarshalV1(conf)
		require.NoError(t, err)

		curveID, err := idemix2.CurveIDFromMSPConfig(&m.MSPConfig{Config: raw})
		require.NoError(t, err)
		require.Equal(t, math.BLS12_381, curveID)
	})

	t.Run("signer curve id fallback", func(t *testing.T) {
		t.Parallel()

		conf := &idemixmsp.IdemixMSPConfig{
			Signer: &idemixmsp.IdemixMSPSignerConfig{CurveId: "gurvy.Bn254"},
		}
		raw, err := proto.MarshalV1(conf)
		require.NoError(t, err)

		curveID, err := idemix2.CurveIDFromMSPConfig(&m.MSPConfig{Config: raw})
		require.NoError(t, err)
		require.Equal(t, math.BN254, curveID)
	})

	t.Run("default curve when missing", func(t *testing.T) {
		t.Parallel()

		conf := &idemixmsp.IdemixMSPConfig{}
		raw, err := proto.MarshalV1(conf)
		require.NoError(t, err)

		curveID, err := idemix2.CurveIDFromMSPConfig(&m.MSPConfig{Config: raw})
		require.NoError(t, err)
		require.Equal(t, math.FP256BN_AMCL, curveID)
	})
}
