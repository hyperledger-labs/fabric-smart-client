/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"os"
	"strings"
	"testing"

	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func TestNewDeserializer(t *testing.T) {
	t.Parallel()

	// Empty IPK skips issuerPublicKey import
	deserializer, err := idemix.NewDeserializer(nil)
	require.NoError(t, err)
	require.NotNil(t, deserializer)

	// Test String()
	require.Contains(t, deserializer.String(), "Idemix with IPK")

	// Test DeserializeSigner (returns not supported error)
	signer, err := deserializer.DeserializeSigner(nil)
	require.ErrorContains(t, err, "not supported")
	require.Nil(t, signer)

	// Test DeserializeVerifier with invalid input
	verifier, err := deserializer.DeserializeVerifier(nil)
	require.Error(t, err)
	require.Nil(t, verifier)

	// Test DeserializeAuditInfo with invalid input
	auditInfo, err := deserializer.DeserializeAuditInfo(nil)
	require.Error(t, err)
	require.Nil(t, auditInfo)

	// Test Info with invalid input
	infoStr, err := deserializer.Info(nil, nil)
	require.Error(t, err)
	require.Empty(t, infoStr)
}

func TestDeserializer_RealIPK(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	_, err := idemix.NewDeserializer([]byte("corrupted-ipk"))
	require.ErrorContains(t, err, "failed to unmarshal issuer public key")

	ipkBytes, err := os.ReadFile("./testdata/idemix/msp/IssuerPublicKey")
	require.NoError(t, err)
	des, err := idemix.NewDeserializer(ipkBytes)
	require.NoError(t, err)
	require.NotNil(t, des.IssuerPublicKey)

	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))
	conf, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix.NewProviderWithEidRhNymPolicy(conf, kvss, sigService)
	require.NoError(t, err)

	id, audit, err := p.Identity(nil)
	require.NoError(t, err)
	// A second identity of the same user: same enrollment ID, different pseudonyms.
	otherID, otherAudit, err := p.Identity(nil)
	require.NoError(t, err)

	t.Run("DeserializeVerifier verifies the identity's signature", func(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		verifier, err := des.DeserializeVerifier(id)
		require.NoError(t, err)
		signer, err := p.DeserializeSigner(id)
		require.NoError(t, err)
		msg := []byte("hello world")
		sigma, err := signer.Sign(msg)
		require.NoError(t, err)
		require.NoError(t, verifier.Verify(msg, sigma))

		// The signature does not verify against another identity's nym.
		otherVerifier, err := des.DeserializeVerifier(otherID)
		require.NoError(t, err)
		require.Error(t, otherVerifier.Verify(msg, sigma))
	})

	t.Run("Info", func(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		infoStr, err := des.Info(id, nil)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(infoStr, "MSP.Idemix: []"), infoStr)
		require.Contains(t, infoStr, "[idemix]")

		_, err = des.Info(id, []byte("garbage-audit-info"))
		require.ErrorContains(t, err, "failed deserializing audit info")

		// Matching audit info yields the enrollment ID, without panicking.
		infoStr, err = des.Info(id, audit)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(infoStr, "MSP.Idemix: [alice]"), infoStr)

		// Audit info of another identity does not match.
		_, err = des.Info(id, otherAudit)
		require.ErrorContains(t, err, "eid nym does not match")

		// Without an issuer public key, as id.Info uses it, audit info cannot be checked.
		noIPK, err := idemix.NewDeserializer(nil)
		require.NoError(t, err)
		_, err = noIPK.Info(id, audit)
		require.ErrorContains(t, err, "error while verifying the nym eid")
	})

	t.Run("DeserializeAgainstNymEID", func(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		ai, err := des.DeserializeAuditInfo(audit)
		require.NoError(t, err)
		require.NotNil(t, ai.EidNymAuditData)
		ident, err := des.DeserializeAgainstNymEID(id, true, ai.EidNymAuditData.Nym.Bytes())
		require.NoError(t, err)
		require.NotNil(t, ident)

		otherAI, err := des.DeserializeAuditInfo(otherAudit)
		require.NoError(t, err)
		_, err = des.DeserializeAgainstNymEID(id, true, otherAI.EidNymAuditData.Nym.Bytes())
		require.ErrorContains(t, err, "cannot deserialize, invalid identity")
	})

	for _, tc := range malformedIdentities(t, id) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		t.Run("DeserializeAgainstNymEID rejects "+tc.name, func(t *testing.T) {
			_, err := des.DeserializeAgainstNymEID(tc.raw, true, nil)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

type malformedIdentity struct {
	name    string
	raw     []byte
	wantErr string
}

// malformedIdentities returns copies of the valid identity, each broken in one field.
func malformedIdentities(t *testing.T, valid []byte) []malformedIdentity {
	t.Helper()
	si := &m.SerializedIdentity{}
	require.NoError(t, proto.Unmarshal(valid, si))
	sIdemix := &m.SerializedIdemixIdentity{}
	require.NoError(t, proto.Unmarshal(si.IdBytes, sIdemix))

	return []malformedIdentity{
		{
			name:    "garbage outer proto",
			raw:     []byte("garbage"),
			wantErr: "failed to unmarshal to msp.SerializedIdentity{}",
		},
		{
			name:    "garbage IdBytes",
			raw:     mustMarshal(t, &m.SerializedIdentity{Mspid: "idemix", IdBytes: []byte("bad-bytes")}),
			wantErr: "could not deserialize a SerializedIdemixIdentity",
		},
		{
			name:    "missing NymX",
			raw:     wrapIdemix(t, &m.SerializedIdemixIdentity{NymY: []byte("nym-y")}),
			wantErr: "unable to deserialize idemix identity: pseudonym is invalid",
		},
		{
			name:    "invalid nym bytes",
			raw:     wrapIdemix(t, &m.SerializedIdemixIdentity{NymX: []byte("bad"), NymY: []byte("bad")}),
			wantErr: "failed to import nym public key",
		},
		{
			name:    "garbage OU",
			raw:     wrapIdemix(t, withField(sIdemix, func(s *m.SerializedIdemixIdentity) { s.Ou = []byte{0xff, 0xff, 0xff} })),
			wantErr: "cannot deserialize the OU of the identity",
		},
		{
			name:    "garbage Role",
			raw:     wrapIdemix(t, withField(sIdemix, func(s *m.SerializedIdemixIdentity) { s.Role = []byte{0xff, 0xff, 0xff} })),
			wantErr: "cannot deserialize the role of the identity",
		},
		{
			name:    "tampered proof",
			raw:     wrapIdemix(t, withField(sIdemix, func(s *m.SerializedIdemixIdentity) { s.Proof = []byte("tampered-proof") })),
			wantErr: "cannot deserialize, invalid identity",
		},
	}
}

// wrapIdemix marshals s into a SerializedIdentity of the "idemix" MSP.
func wrapIdemix(t *testing.T, s *m.SerializedIdemixIdentity) []byte {
	t.Helper()
	return mustMarshal(t, &m.SerializedIdentity{Mspid: "idemix", IdBytes: mustMarshal(t, s)})
}

func mustMarshal(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(msg)
	require.NoError(t, err)
	return raw
}

// withField returns a copy of s with mutate applied.
func withField(s *m.SerializedIdemixIdentity, mutate func(*m.SerializedIdemixIdentity)) *m.SerializedIdemixIdentity {
	c := proto.Clone(s).(*m.SerializedIdemixIdentity)
	mutate(c)
	return c
}
