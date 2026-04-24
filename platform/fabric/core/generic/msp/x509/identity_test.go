/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"encoding/pem"
	"path/filepath"
	"testing"

	msp2 "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

func TestSerialize(t *testing.T) {
	t.Parallel()
	certPath := filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	raw, err := Serialize("apple", certPath)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	si := &msp2.SerializedIdentity{}
	require.NoError(t, proto.Unmarshal(raw, si))
	require.Equal(t, "apple", si.Mspid)
}

func TestSerialize_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := Serialize("apple", "/nonexistent/path/cert.pem")
	require.Error(t, err)
}

func TestSerializeRaw(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	raw, err := SerializeRaw("testmsp", certPEM)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestSerializeRaw_InvalidPEM(t *testing.T) {
	t.Parallel()
	_, err := SerializeRaw("testmsp", []byte("not a cert"))
	require.Error(t, err)
}

func TestSerializeRaw_NilBytes(t *testing.T) {
	t.Parallel()
	_, err := SerializeRaw("testmsp", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil idBytes")
}

func TestGetCertFromPem_Errors(t *testing.T) {
	t.Parallel()
	_, err := getCertFromPem(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil idBytes")

	_, err = getCertFromPem([]byte("not pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not decode pem bytes")

	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	_, err = getCertFromPem(badCert)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse x509 cert")
}
