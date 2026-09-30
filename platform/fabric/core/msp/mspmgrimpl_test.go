/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// renamedMSP overrides the identifier of the wrapped MSP, so the manager
// treats it as an MSP of an unknown implementation type.
type renamedMSP struct {
	MSP
	name string
	err  error
}

func (r *renamedMSP) GetIdentifier() (string, error) { return r.name, r.err }

func TestMSPManagerSetupFailsOnIdentifier(t *testing.T) { //nolint:paralleltest
	mgr := NewMSPManager()
	err := mgr.Setup([]MSP{&renamedMSP{MSP: localMsp, err: errors.New("no identifier")}})
	require.ErrorContains(t, err, "could not extract msp identifier")
}

func TestMSPManagerDeserializeIdentity(t *testing.T) { //nolint:paralleltest
	mgr := NewMSPManager()
	_, err := mgr.DeserializeIdentity(nil)
	require.EqualError(t, err, "channel doesn't exist")

	idemixMSP := newLocalIdemixMSP(t)
	require.NoError(t, mgr.Setup([]MSP{localMsp, idemixMSP, &renamedMSP{MSP: localMsp, name: "renamed"}}))

	idemixSigner, err := idemixMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	raw, err := idemixSigner.Serialize()
	require.NoError(t, err)
	id, err := mgr.DeserializeIdentity(raw)
	require.NoError(t, err)
	require.IsType(t, &idemixIdentityWrapper{}, id)

	x509Signer, err := localMsp.GetDefaultSigningIdentity()
	require.NoError(t, err)
	raw, err = x509Signer.Serialize()
	require.NoError(t, err)
	id, err = mgr.DeserializeIdentity(raw)
	require.NoError(t, err)
	require.IsType(t, &identity{}, id)

	// MSPs of other types receive the full serialized identity, so the
	// wrapped MSP sees the manager-level MSP ID and rejects it.
	raw, err = proto.Marshal(&msp.SerializedIdentity{Mspid: "renamed", IdBytes: x509Signer.(*signingidentity).cert.Raw})
	require.NoError(t, err)
	_, err = mgr.DeserializeIdentity(raw)
	require.ErrorContains(t, err, "expected MSP ID SampleOrg, received renamed")

	_, err = mgr.DeserializeIdentity([]byte{0xff})
	require.ErrorContains(t, err, "could not deserialize a SerializedIdentity")

	raw, err = proto.Marshal(&msp.SerializedIdentity{Mspid: "unknown"})
	require.NoError(t, err)
	_, err = mgr.DeserializeIdentity(raw)
	require.EqualError(t, err, "MSP unknown is not defined on channel")
}
