/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// A recipient proof made by a real idemix pseudonym verifies with the idemix
// verifier for that pseudonym, and only for the context it was made for.
func TestRecipientProof_Idemix(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	d := mem.NewDriver()
	store, err := d.NewKVS("")
	require.NoError(t, err)
	kvss, err := kvs.New(store, "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	auditInfo, err := d.NewAuditInfo("")
	require.NoError(t, err)
	signerInfo, err := d.NewSignerInfo("")
	require.NoError(t, err)
	conf, err := fabricmsp.GetLocalMspConfigWithType("../../core/generic/msp/idemix/testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	provider, err := idemix.NewProviderWithAnyPolicy(conf, kvss, sig.NewService(sig.NewMultiplexDeserializer(), auditInfo, signerInfo))
	require.NoError(t, err)

	x, _, err := provider.Identity(nil)
	require.NoError(t, err)
	signer, err := provider.DeserializeSigner(x)
	require.NoError(t, err)
	verifier, err := provider.DeserializeVerifier(x)
	require.NoError(t, err)

	p := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     x,
	}
	proof, err := SignRecipientProof(signer, p)
	require.NoError(t, err)
	require.NoError(t, VerifyRecipientProof(verifier, p, proof))

	relayed := p
	relayed.VerifierNode = view.Identity("mallory")
	require.ErrorIs(t, VerifyRecipientProof(verifier, relayed, proof), ErrRecipientProof)
}
