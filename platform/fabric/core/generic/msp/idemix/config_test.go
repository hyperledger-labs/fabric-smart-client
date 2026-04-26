/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"testing"

	"github.com/IBM/idemix/idemixmsp"
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
