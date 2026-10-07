/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"testing"

	idemixconfig "github.com/IBM/idemix/msp/config"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

func TestStampCurveID(t *testing.T) {
	orig := &idemixconfig.IdemixMSPConfig{Name: "idemix"}
	raw, err := proto.Marshal(orig)
	require.NoError(t, err)
	conf := &m.MSPConfig{Config: raw}

	require.NoError(t, stampCurveID(conf, "BN254"))

	var stamped idemixconfig.IdemixMSPConfig
	require.NoError(t, proto.Unmarshal(conf.Config, &stamped))
	require.Equal(t, "BN254", stamped.CurveId)
	require.Equal(t, "idemix", stamped.Name)
}

func TestStampCurveID_UnmarshalError(t *testing.T) {
	conf := &m.MSPConfig{Config: []byte("not a valid protobuf payload")}
	err := stampCurveID(conf, "BN254")
	require.ErrorContains(t, err, "failed unmarshalling idemix msp config")
}
