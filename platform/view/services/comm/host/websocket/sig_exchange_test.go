/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	idecdsa "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestSignedSessionExchange wires the comm stack together with the endpoint and
// sig services over the websocket transport, and runs the signed message
// exchange between Alice and Bob. Each node signs with the ECDSA secret key of
// the very key-pair (its TLS certificate/key) used to instantiate its comm
// stack, and the counterparty verifies using the public key resolved from the
// session's RemotePKID.
func TestSignedSessionExchange(t *testing.T) { //nolint:paralleltest
	tls := generateThreeNodesTLSFiles(t)
	aliceNode, bobNode := setupTwoNodesFromTLS(t, tls.alice, tls.bob, tls.caCert)

	alice := newWebsocketSignedParty(t, "alice", aliceNode, tls.alice, bobNode, tls.bob)
	bob := newWebsocketSignedParty(t, "bob", bobNode, tls.bob, aliceNode, tls.alice)

	comm.SignedSessionExchangeTestRound(t, alice, bob)
}

// newWebsocketSignedParty builds a SignedExchangeParty for the given node.
//
// self/selfTLS identify the local node; remote/remoteTLS identify the peer. Both
// are registered as resolvers in the local endpoint service (so the local node
// can resolve the peer's PKID), and the local signer is registered in the local
// sig service using the node's own certificate as identity.
func newWebsocketSignedParty(
	t *testing.T,
	name string,
	self *comm.HostNode,
	selfTLS nodeTLSFiles,
	remote *comm.HostNode,
	remoteTLS nodeTLSFiles,
) *comm.SignedExchangeParty {
	t.Helper()
	ctx := context.Background()

	selfCert := mustReadFile(t, selfTLS.cert)
	remoteCert := mustReadFile(t, remoteTLS.cert)

	// --- endpoint service ---------------------------------------------------
	drv := mem.NewDriver()
	bindingStore := utils.MustGet(drv.NewBinding(""))
	es, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)
	// Use the same PKI extractor and synthesizer as the production websocket
	// provider, so PKIDs computed here match the ones the comm layer reports.
	require.NoError(t, es.AddPublicKeyExtractor(&comm.PKExtractor{}))
	es.SetPublicKeyIDSynthesizer(&websocket.PKIDSynthesizer{})

	// Register both parties. The identity of each resolver is its certificate;
	// the endpoint service indexes it by the synthesized PKID as well, which is
	// what SessionInfo.RemotePKID carries.
	_, err = es.AddResolver(name, "", map[string]string{string(endpoint.P2PPort): self.Address}, nil, selfCert)
	require.NoError(t, err)
	_, err = es.AddResolver("remote-"+name, "", map[string]string{string(endpoint.P2PPort): remote.Address}, nil, remoteCert)
	require.NoError(t, err)

	// --- sig service --------------------------------------------------------
	deserializer, err := sig.NewDeserializer()
	require.NoError(t, err)
	signerStore := utils.MustGet(drv.NewSignerInfo(""))
	auditStore := utils.MustGet(drv.NewAuditInfo(""))
	sigService := sig.NewService(deserializer, auditStore, signerStore)

	// Register this node's signer using the secret key of its comm key-pair.
	signer, err := idecdsa.NewSignerFromPEM(mustReadFile(t, selfTLS.key))
	require.NoError(t, err)
	require.NoError(t, sigService.RegisterSigner(ctx, selfCert, signer, nil))

	return &comm.SignedExchangeParty{
		Name: name,
		Node: self,
		Sign: func(localPKID, payload []byte) ([]byte, error) {
			// Select the local identity (and thus the signer) from the session's
			// LocalPKID, symmetrically to how the verifier is selected from
			// RemotePKID.
			localID, err := es.GetIdentity("", localPKID)
			if err != nil {
				return nil, err
			}
			s, err := sigService.GetSigner(localID)
			if err != nil {
				return nil, err
			}
			return s.Sign(payload)
		},
		ResolveIdentity: func(pkid []byte) (view.Identity, error) {
			return es.GetIdentity("", pkid)
		},
		Verify: func(identity view.Identity, payload, signature []byte) error {
			v, err := sigService.GetVerifier(identity)
			if err != nil {
				return err
			}
			return v.Verify(payload, signature)
		},
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}
