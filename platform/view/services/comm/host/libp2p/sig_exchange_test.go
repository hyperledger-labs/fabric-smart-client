/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	idecdsa "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// libp2pKey bundles everything derived from a node's key-pair that the signed
// exchange needs: the raw ECDSA private key (to sign), the PKIX-encoded public
// key (used as the node's identity), the libp2p private key (to instantiate the
// host), and the libp2p peer ID string (which is what the comm layer reports as
// the PKID).
type libp2pKey struct {
	priv    *ecdsa.PrivateKey
	pkixPub []byte
	cryptoK crypto.PrivKey
	peerID  string
}

func newLibp2pKey(t *testing.T) libp2pKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pkixPub, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	cryptoK, cryptoPub, err := crypto.ECDSAKeyPairFromKey(priv)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(cryptoPub)
	require.NoError(t, err)
	return libp2pKey{priv: priv, pkixPub: pkixPub, cryptoK: cryptoK, peerID: id.String()}
}

// ecdsaPKExtractor extracts the ECDSA public key from an identity that is the
// PKIX-encoded public key. It is the libp2p counterpart of comm.PKExtractor
// (which expects an x509 certificate).
type ecdsaPKExtractor struct{}

func (ecdsaPKExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	return x509.ParsePKIXPublicKey(id)
}

// ecdsaSigDeserializer turns a PKIX-encoded public key identity into a verifier.
// Signers are never deserialized in this test (they are registered explicitly).
type ecdsaSigDeserializer struct{}

func (ecdsaSigDeserializer) DeserializeVerifier(raw []byte) (cdriver.Verifier, error) {
	_, v, err := idecdsa.NewIdentityFromBytes(raw)
	return v, err
}

func (ecdsaSigDeserializer) DeserializeSigner(raw []byte) (cdriver.Signer, error) {
	return nil, nil
}

func (ecdsaSigDeserializer) Info(raw, auditInfo []byte) (string, error) {
	return view.Identity(raw).UniqueID(), nil
}

// TestSignedSessionExchange wires the comm stack together with the endpoint and
// sig services over the libp2p transport, and runs the signed message exchange
// between Alice and Bob. Each node signs with the ECDSA secret key of the very
// key-pair used to instantiate its libp2p host, and the counterparty verifies
// using the public key resolved from the session's RemotePKID.
func TestSignedSessionExchange(t *testing.T) { //nolint:paralleltest
	aliceKey := newLibp2pKey(t)
	bobKey := newLibp2pKey(t)

	addrs := freeLibP2PAddresses(t, 2)
	aliceAddr := addrs[0]
	bobAddr := addrs[1]

	// Alice is the bootstrap node; Bob connects to Alice.
	aliceCfg := &mock.LibP2PConfig{}
	aliceCfg.ListenAddressReturns(aliceAddr)
	aliceHost, err := newLibP2PHost(aliceCfg, aliceKey.cryptoK, newMetrics(&disabled.Provider{}), true, "")
	require.NoError(t, err)
	aliceP2P, err := comm.NewNode(t.Context(), aliceHost, &disabled.Provider{})
	require.NoError(t, err)

	bobCfg := &mock.LibP2PConfig{}
	bobCfg.ListenAddressReturns(bobAddr)
	bobHost, err := newLibP2PHost(bobCfg, bobKey.cryptoK, newMetrics(&disabled.Provider{}), false, aliceAddr+"/p2p/"+aliceKey.peerID)
	require.NoError(t, err)
	bobP2P, err := comm.NewNode(t.Context(), bobHost, &disabled.Provider{})
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	aliceNode := &comm.HostNode{P2PNode: aliceP2P, ID: aliceKey.peerID, Address: aliceAddr}
	bobNode := &comm.HostNode{P2PNode: bobP2P, ID: bobKey.peerID, Address: bobAddr}

	alice := newLibp2pSignedParty(t, "alice", aliceNode, aliceKey, bobNode, bobKey)
	bob := newLibp2pSignedParty(t, "bob", bobNode, bobKey, aliceNode, aliceKey)

	comm.SignedSessionExchangeTestRound(t, alice, bob)
}

func newLibp2pSignedParty(
	t *testing.T,
	name string,
	self *comm.HostNode,
	selfKey libp2pKey,
	remote *comm.HostNode,
	remoteKey libp2pKey,
) *comm.SignedExchangeParty {
	t.Helper()
	ctx := context.Background()

	// --- endpoint service ---------------------------------------------------
	// nil binding store: the test never triggers a binding lookup, and passing
	// nil keeps this (separate) module free of the storage/sqlite dependency.
	es, err := endpoint.NewService(nil)
	require.NoError(t, err)
	require.NoError(t, es.AddPublicKeyExtractor(ecdsaPKExtractor{}))
	es.SetPublicKeyIDSynthesizer(PKIDSynthesizer{})

	_, err = es.AddResolver(name, "", map[string]string{string(endpoint.P2PPort): self.Address}, nil, selfKey.pkixPub)
	require.NoError(t, err)
	_, err = es.AddResolver("remote-"+name, "", map[string]string{string(endpoint.P2PPort): remote.Address}, nil, remoteKey.pkixPub)
	require.NoError(t, err)

	// --- sig service --------------------------------------------------------
	// nil stores: signer/verifier are exercised entirely from the in-memory
	// caches populated by RegisterSigner / GetVerifier deserialization.
	sigService := sig.NewService(ecdsaSigDeserializer{}, nil, nil)

	signer := &idecdsa.Signer{PrivateKey: selfKey.priv}
	require.NoError(t, sigService.RegisterSigner(ctx, selfKey.pkixPub, signer, nil))

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
