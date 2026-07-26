/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/utils"
	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	mspx509 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	comm "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var (
	ctx = context.Background()

	expectedOrgCombinations = []map[string]struct{}{
		{
			"A": {},
		},
	}

	cc = &gossip.Chaincode{
		Name:    "mycc",
		Version: "1.0",
	}

	expectedConf = &discovery.ConfigResult{
		Msps: map[string]*msp.FabricMSPConfig{
			"A": {},
			"B": {},
			"C": {},
			"D": {},
		},
		Orderers: map[string]*discovery.Endpoints{
			"A": {},
			"B": {},
		},
	}

	resultsWithoutEnvelopes = &discovery.QueryResult_CcQueryRes{
		CcQueryRes: &discovery.ChaincodeQueryResult{
			Content: []*discovery.EndorsementDescriptor{
				{
					Chaincode: "mycc",
					EndorsersByGroups: map[string]*discovery.Peers{
						"A": {
							Peers: []*discovery.Peer{
								{},
							},
						},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{},
						},
					},
				},
			},
		},
	}

	resultsWithEnvelopesButWithInsufficientPeers = &discovery.QueryResult_CcQueryRes{
		CcQueryRes: &discovery.ChaincodeQueryResult{
			Content: []*discovery.EndorsementDescriptor{
				{
					Chaincode: "mycc",
					EndorsersByGroups: map[string]*discovery.Peers{
						"A": {
							Peers: []*discovery.Peer{
								{
									StateInfo:      stateInfoMessage(),
									MembershipInfo: aliveMessage(0),
									Identity:       peerIdentity("A", 0),
								},
							},
						},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{
								"A": 2,
							},
						},
					},
				},
			},
		},
	}

	resultsWithEnvelopesButWithMismatchedLayout = &discovery.QueryResult_CcQueryRes{
		CcQueryRes: &discovery.ChaincodeQueryResult{
			Content: []*discovery.EndorsementDescriptor{
				{
					Chaincode: "mycc",
					EndorsersByGroups: map[string]*discovery.Peers{
						"A": {
							Peers: []*discovery.Peer{
								{
									StateInfo:      stateInfoMessage(),
									MembershipInfo: aliveMessage(0),
									Identity:       peerIdentity("A", 0),
								},
							},
						},
					},
					Layouts: []*discovery.Layout{
						{
							QuantitiesByGroup: map[string]uint32{
								"B": 2,
							},
						},
					},
				},
			},
		},
	}
)

func loadFileOrPanic(file string) []byte {
	b, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}
	return b
}

func createConnector(t *testing.T, certificate tls.Certificate, targetPort int) func() (*grpc.ClientConn, error) {
	t.Helper()
	caCert := loadFileOrPanic(filepath.Join("testdata", "server", "ca.pem"))
	tlsConf := &tls.Config{
		RootCAs:      x509.NewCertPool(),
		Certificates: []tls.Certificate{certificate},
	}
	tlsConf.RootCAs.AppendCertsFromPEM(caCert)

	addr := fmt.Sprintf("localhost:%d", targetPort)
	return func() (*grpc.ClientConn, error) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
		require.NoError(t, err)
		return conn, nil
	}
}

func TestClient(t *testing.T) {
	t.Parallel()
	clientCert := loadFileOrPanic(filepath.Join("testdata", "client", "cert.pem"))
	clientKey := loadFileOrPanic(filepath.Join("testdata", "client", "key.pem"))
	clientTLSCert, err := tls.X509KeyPair(clientCert, clientKey)
	require.NoError(t, err)

	svc := newMockDiscoveryService()
	port := svc.port
	connect := createConnector(t, clientTLSCert, int(port))

	signer := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	authInfo := &discovery.AuthInfo{
		ClientIdentity:    []byte{1, 2, 3},
		ClientTlsCertHash: computeSHA256(clientTLSCert.Certificate[0]),
	}
	cl := NewClient(connect, signer)

	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: &discovery.QueryResult_Members{
					Members: &discovery.PeerMembershipResult{
						PeersByOrg: map[string]*discovery.Peers{
							"A": {
								Peers: []*discovery.Peer{
									{
										StateInfo:      stateInfoMessage(),
										MembershipInfo: aliveMessage(0),
										Identity:       peerIdentity("A", 0),
									},
									{
										StateInfo:      stateInfoMessage(),
										MembershipInfo: aliveMessage(1),
										Identity:       peerIdentity("A", 1),
									},
								},
							},
							"B": {
								Peers: []*discovery.Peer{
									{
										StateInfo:      stateInfoMessage(),
										MembershipInfo: aliveMessage(0),
										Identity:       peerIdentity("B", 0),
									},
								},
							},
							"C": {
								Peers: []*discovery.Peer{
									{
										StateInfo:      stateInfoMessage(),
										MembershipInfo: aliveMessage(0),
										Identity:       peerIdentity("C", 0),
									},
								},
							},
						},
					},
				},
			},
			{
				Result: &discovery.QueryResult_ConfigResult{
					ConfigResult: expectedConf,
				},
			},
			{
				Result: &discovery.QueryResult_Error{
					Error: &discovery.Error{
						Content: "failed constructing descriptor for chaincode",
					},
				},
			},
		},
	}, nil).Once()

	// sup.On("PeersOfChannel").Return(channelPeersWithoutChaincodes).Times(2)
	req := NewRequest()
	req, err = req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err := cl.Send(ctx, req, authInfo)
	require.NoError(t, err)

	t.Run("Channel mismatch", func(t *testing.T) {
		t.Parallel()
		// Check behavior for channels that we didn't query for.
		fakeChannel := r.ForChannel("fakeChannel")
		peers, err := fakeChannel.Peers()
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, peers)

		endorsers, err := fakeChannel.Endorsers(ccCall("mycc"), NoFilter)
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, endorsers)

		conf, err := fakeChannel.Config()
		require.Equal(t, ErrNotFound, err)
		require.Nil(t, conf)
	})

	t.Run("Peer membership query", func(t *testing.T) {
		t.Parallel()
		// Check response for the correct channel
		mychannel := r.ForChannel("mychannel")
		conf, err := mychannel.Config()
		require.NoError(t, err)
		require.True(t, proto.Equal(expectedConf, conf))
		peers, err := mychannel.Peers()
		require.NoError(t, err)
		// We should see all peers as provided above
		require.Len(t, peers, 4)
	})

	t.Run("Endorser query without chaincode installed", func(t *testing.T) {
		t.Parallel()
		mychannel := r.ForChannel("mychannel")
		endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
		// However, since we didn't provide any chaincodes to these peers - the server shouldn't
		// be able to construct the descriptor.
		// Just check that the appropriate error is returned, and nothing crashes.
		require.Contains(t, err.Error(), "failed constructing descriptor for chaincode")
		require.Nil(t, endorsers)
	})

	t.Run("Endorser query with chaincodes installed", func(t *testing.T) {
		t.Parallel()
		// Next, we check the case when the peers publish chaincode for themselves.
		// TODO: produce output

		svc.On("Discover").Return(&discovery.Response{
			Results: []*discovery.QueryResult{
				{
					Result: &discovery.QueryResult_Members{
						Members: &discovery.PeerMembershipResult{
							PeersByOrg: map[string]*discovery.Peers{
								"A": {
									Peers: []*discovery.Peer{
										{
											StateInfo:      stateInfoMessage(cc),
											MembershipInfo: aliveMessage(0),
											Identity:       peerIdentity("A", 0),
										},
									},
								},
								"B": {
									Peers: []*discovery.Peer{
										{
											StateInfo:      stateInfoMessage(cc),
											MembershipInfo: aliveMessage(0),
											Identity:       peerIdentity("B", 1),
										},
									},
								},
								"C": {
									Peers: []*discovery.Peer{
										{
											StateInfo:      stateInfoMessage(cc),
											MembershipInfo: aliveMessage(0),
											Identity:       peerIdentity("C", 3),
										},
									},
								},
							},
						},
					},
				},
				{
					Result: &discovery.QueryResult_CcQueryRes{
						CcQueryRes: &discovery.ChaincodeQueryResult{
							Content: []*discovery.EndorsementDescriptor{
								{
									Chaincode: "mycc",
									EndorsersByGroups: map[string]*discovery.Peers{
										"A": {
											Peers: []*discovery.Peer{
												{
													StateInfo:      stateInfoMessage(cc),
													MembershipInfo: aliveMessage(0),
													Identity:       peerIdentity("A", 0),
												},
											},
										},
									},
									Layouts: []*discovery.Layout{
										{
											QuantitiesByGroup: map[string]uint32{
												"A": 1,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}).Once()

		req = NewRequest()
		req, err = req.OfChannel("mychannel").AddPeersQuery().AddEndorsersQuery(interest("mycc"))
		require.NoError(t, err)
		r2, err := cl.Send(ctx, req, authInfo)
		require.NoError(t, err)

		mychannel := r2.ForChannel("mychannel")
		peers, err := mychannel.Peers()
		require.NoError(t, err)
		require.Len(t, peers, 3)

		// We should get a valid endorsement descriptor from the service
		endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
		require.NoError(t, err)
		// The combinations of endorsers should be in the expected combinations
		require.Contains(t, expectedOrgCombinations, getMSPs(endorsers))
	})
}

func computeSHA256(bytes []byte) []byte {
	h := sha256.Sum256(bytes)
	return h[:]
}

func TestUnableToSign(t *testing.T) {
	t.Parallel()
	signer := func(msg []byte) ([]byte, error) {
		return nil, errors.New("not enough entropy")
	}
	failToConnect := func() (*grpc.ClientConn, error) {
		return nil, nil
	}
	authInfo := &discovery.AuthInfo{
		ClientIdentity: []byte{1, 2, 3},
	}
	cl := NewClient(failToConnect, signer)
	req := NewRequest()
	req = req.OfChannel("mychannel")
	resp, err := cl.Send(ctx, req, authInfo)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "not enough entropy")
}

func TestUnableToConnect(t *testing.T) {
	t.Parallel()
	signer := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	failToConnect := func() (*grpc.ClientConn, error) {
		return nil, errors.New("unable to connect")
	}
	auth := &discovery.AuthInfo{
		ClientIdentity: []byte{1, 2, 3},
	}
	cl := NewClient(failToConnect, signer)
	req := NewRequest()
	req = req.OfChannel("mychannel")
	resp, err := cl.Send(ctx, req, auth)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "unable to connect")
}

func TestBadResponses(t *testing.T) {
	t.Parallel()
	signer := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	svc := newMockDiscoveryService()
	t.Logf("Started mock discovery service on port %d", svc.port)
	defer svc.shutdown()

	clientCert := loadFileOrPanic(filepath.Join("testdata", "client", "cert.pem"))
	clientKey := loadFileOrPanic(filepath.Join("testdata", "client", "key.pem"))
	clientTLSCert, err := tls.X509KeyPair(clientCert, clientKey)
	require.NoError(t, err)
	connect := createConnector(t, clientTLSCert, int(svc.port))

	auth := &discovery.AuthInfo{
		ClientIdentity: []byte{1, 2, 3},
	}
	cl := NewClient(connect, signer)

	// Scenario I: discovery service sends back an error
	svc.On("Discover").Return(nil, errors.New("foo")).Once()
	req := NewRequest()
	req, err = req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err := cl.Send(ctx, req, auth)
	require.Contains(t, err.Error(), "foo")
	require.Nil(t, r)

	// Scenario II: discovery service sends back an empty response
	svc.On("Discover").Return(&discovery.Response{}, nil).Once()
	req = NewRequest()
	req, err = req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err = cl.Send(ctx, req, auth)
	require.Equal(t, "Sent 3 queries but received 0 responses back", err.Error())
	require.Nil(t, r)

	// Scenario III: discovery service sends back a layout for the wrong chaincode
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: &discovery.QueryResult_CcQueryRes{
					CcQueryRes: &discovery.ChaincodeQueryResult{
						Content: []*discovery.EndorsementDescriptor{
							{
								Chaincode: "notmycc",
							},
						},
					},
				},
			},
		},
	}, nil).Once()
	req = NewRequest()
	req, err = req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err = cl.Send(ctx, req, auth)
	require.Nil(t, r)
	require.Contains(t, err.Error(), "expected chaincode mycc but got endorsement descriptor for notmycc")

	// Scenario IV: discovery service sends back a layout that has empty envelopes
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: resultsWithoutEnvelopes,
			},
		},
	}, nil).Once()
	req = NewRequest()
	req, err = req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err = cl.Send(ctx, req, auth)
	require.Contains(t, err.Error(), "received empty envelope(s) for endorsers for chaincode mycc")
	require.Nil(t, r)

	// Scenario V: discovery service sends back a layout that has a group that requires more
	// members than are present.
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: resultsWithEnvelopesButWithInsufficientPeers,
			},
		},
	}, nil).Once()
	req = NewRequest()
	req, err = req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err = cl.Send(ctx, req, auth)
	require.NoError(t, err)
	mychannel := r.ForChannel("mychannel")
	endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
	require.Nil(t, endorsers)
	require.Contains(t, err.Error(), "no endorsement combination can be satisfied")

	// Scenario VI: discovery service sends back a layout that has a group that doesn't have a matching peer set
	svc.On("Discover").Return(&discovery.Response{
		Results: []*discovery.QueryResult{
			{
				Result: resultsWithEnvelopesButWithMismatchedLayout,
			},
		},
	}, nil).Once()
	req = NewRequest()
	req, err = req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	require.NoError(t, err)
	r, err = cl.Send(ctx, req, auth)
	require.Contains(t, err.Error(), "group B isn't mapped to endorsers, but exists in a layout")
	require.Empty(t, r)
}

func TestAddEndorsersQueryInvalidInput(t *testing.T) {
	t.Parallel()
	_, err := NewRequest().AddEndorsersQuery()
	require.Contains(t, err.Error(), "no chaincode interests given")

	_, err = NewRequest().AddEndorsersQuery(nil)
	require.Contains(t, err.Error(), "chaincode interest is nil")

	_, err = NewRequest().AddEndorsersQuery(&peer.ChaincodeInterest{})
	require.Contains(t, err.Error(), "invocation chain should not be empty")

	_, err = NewRequest().AddEndorsersQuery(&peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{{}},
	})
	require.Contains(t, err.Error(), "chaincode name should not be empty")
}

func TestValidateAliveMessage(t *testing.T) {
	t.Parallel()
	am := aliveMessage(1)
	msg, _ := EnvelopeToGossipMessage(am)

	// Scenario I: Valid alive message
	require.NoError(t, validateAliveMessage(msg))

	// Scenario II: Nullify timestamp
	msg.GetAliveMsg().Timestamp = nil
	err := validateAliveMessage(msg)
	require.Equal(t, "timestamp is nil", err.Error())

	// Scenario III: Nullify membership
	msg.GetAliveMsg().Membership = nil
	err = validateAliveMessage(msg)
	require.Equal(t, "membership is empty", err.Error())

	// Scenario IV: Nullify the entire alive message part
	msg.Content = nil
	err = validateAliveMessage(msg)
	require.Equal(t, "message isn't an alive message", err.Error())
}

func TestValidateStateInfoMessage(t *testing.T) {
	t.Parallel()
	si := stateInfoWithHeight(100)

	// Scenario I: Valid state info message
	require.NoError(t, validateStateInfoMessage(si))

	// Scenario II: Nullify properties
	si.GetStateInfo().Properties = nil
	err := validateStateInfoMessage(si)
	require.Equal(t, "properties is nil", err.Error())

	// Scenario III: Nullify timestamp
	si.GetStateInfo().Timestamp = nil
	err = validateStateInfoMessage(si)
	require.Equal(t, "timestamp is nil", err.Error())

	// Scenario IV: Nullify the state info message part
	si.Content = nil
	err = validateStateInfoMessage(si)
	require.Equal(t, "message isn't a stateInfo message", err.Error())
}

// TestForgedGossipEnvelopeRejected is a security regression test for a fix
// to a bug where EnvelopeToGossipMessage and
// validateAliveMessage/validateStateInfoMessage never checked
// gossip.Envelope.Signature against gossip.Envelope.Payload. A
// discovery-service peer that relays Alive/StateInfo entries collected from
// gossip (peersForChannel, endorser in client.go) could therefore forge the
// Membership.Endpoint of any org's peer - even one it never received a
// genuine envelope for - and have it accepted as valid, since there was no
// signature verification anywhere in the call chain. This held even when the
// discovery gRPC connection itself was mTLS-authenticated: mTLS only proves
// who you're talking to, not whether the gossip payload they're relaying was
// honestly produced by the peer it claims to be from.
//
// peersForChannel now verifies the envelope's signature against the public
// key embedded in the peer's own claimed Identity before trusting its
// content, so both a garbage signature and a signature produced by the
// wrong key (i.e. an attacker who doesn't hold the claimed identity's
// private key) are rejected.
func TestForgedGossipEnvelopeRejected(t *testing.T) {
	t.Parallel()

	genuine := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{
				Timestamp:  &gossip.PeerTime{SeqNum: 1, IncNum: 1},
				Membership: &gossip.Member{Endpoint: "real-peer.example.com:7051"},
			},
		},
	}
	payload, err := proto.Marshal(genuine)
	require.NoError(t, err)

	membersResFor := func(envelope *gossip.Envelope, identity []byte) *discovery.PeerMembershipResult {
		return &discovery.PeerMembershipResult{
			PeersByOrg: map[string]*discovery.Peers{
				"A": {
					Peers: []*discovery.Peer{
						{
							MembershipInfo: envelope,
							StateInfo:      stateInfoMessage(),
							Identity:       identity,
						},
					},
				},
			},
		}
	}

	t.Run("garbage signature", func(t *testing.T) {
		t.Parallel()
		forged := &gossip.Envelope{
			Payload:   payload,
			Signature: []byte("not-a-real-signature-from-anyone"),
		}
		_, err := peersForChannel(membersResFor(forged, peerIdentity("A", 0)), PeerMembershipQueryType)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed verifying alive message signature")
	})

	t.Run("signature from the wrong key", func(t *testing.T) {
		t.Parallel()
		attackerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		digest := sha256.Sum256(payload)
		r, s, err := ecdsa.Sign(rand.Reader, attackerKey, digest[:])
		require.NoError(t, err)
		s, _, err = mspx509.ToLowS(&attackerKey.PublicKey, s)
		require.NoError(t, err)
		sig, err := utils.MarshalECDSASignature(r, s)
		require.NoError(t, err)

		forged := &gossip.Envelope{
			Payload:   payload,
			Signature: sig,
		}
		// Identity claims to be peer "A"/0 (whose key is testPeerKey), but the
		// signature was produced by an unrelated attacker key.
		_, err = peersForChannel(membersResFor(forged, peerIdentity("A", 0)), PeerMembershipQueryType)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed verifying alive message signature")
	})

	t.Run("genuine signature is accepted", func(t *testing.T) {
		t.Parallel()
		sig, err := signWithTestPeerKey(payload)
		require.NoError(t, err)
		genuineEnvelope := &gossip.Envelope{
			Payload:   payload,
			Signature: sig,
		}
		peers, err := peersForChannel(membersResFor(genuineEnvelope, peerIdentity("A", 0)), PeerMembershipQueryType)
		require.NoError(t, err)
		require.Len(t, peers, 1)
		require.Equal(t, "real-peer.example.com:7051", peers[0].AliveMessage.GetAliveMsg().Membership.Endpoint)
	})

	// A real Fabric peer signs its own self-referential AliveMessage with
	// protoext.NoopSign (gossip/discovery's Self()), which produces a nil
	// Signature by design, and the Discovery service reports that self-entry
	// as-is whenever the responding peer is itself among the reported peers -
	// the common case in small test networks, where the peer answering the
	// query is very often also one of the few endorsers/members it reports
	// on. This must be accepted, not treated as a forged/garbage signature.
	t.Run("nil alive message signature from a self-entry is accepted", func(t *testing.T) {
		t.Parallel()
		selfEnvelope := &gossip.Envelope{
			Payload:   payload,
			Signature: nil,
		}
		peers, err := peersForChannel(membersResFor(selfEnvelope, peerIdentity("A", 0)), PeerMembershipQueryType)
		require.NoError(t, err)
		require.Len(t, peers, 1)
		require.Equal(t, "real-peer.example.com:7051", peers[0].AliveMessage.GetAliveMsg().Membership.Endpoint)
	})

	// Unlike AliveMessage, a real peer's StateInfo self-entry is always
	// genuinely signed (gossipChannel.setupSignedStateInfoMessage calls a
	// real signer, not NoopSign), so a nil StateInfo signature has no
	// legitimate source and must still be rejected.
	t.Run("nil stateInfo signature is still rejected", func(t *testing.T) {
		t.Parallel()
		sig, err := signWithTestPeerKey(payload)
		require.NoError(t, err)
		genuineAlive := &gossip.Envelope{
			Payload:   payload,
			Signature: sig,
		}
		membersRes := membersResFor(genuineAlive, peerIdentity("A", 0))
		membersRes.PeersByOrg["A"].Peers[0].StateInfo = &gossip.Envelope{
			Payload:   stateInfoMessage().Payload,
			Signature: nil,
		}
		_, err = peersForChannel(membersRes, PeerMembershipQueryType)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed verifying stateInfo message signature")
	})
}

func TestString(t *testing.T) {
	t.Parallel()
	var ic InvocationChain
	ic = append(ic, &peer.ChaincodeCall{
		Name:            "foo",
		CollectionNames: []string{"c1", "c2"},
	})
	ic = append(ic, &peer.ChaincodeCall{
		Name:            "bar",
		CollectionNames: []string{"c3", "c4"},
	})
	expected := `[{"name":"foo","collection_names":["c1","c2"]},{"name":"bar","collection_names":["c3","c4"]}]`
	require.Equal(t, expected, ic.String())
}

func getMSP(peer *Peer) string {
	endpoint := peer.AliveMessage.GetAliveMsg().Membership.Endpoint
	id, _ := strconv.ParseInt(endpoint[1:], 10, 64)
	switch id / 2 {
	case 0, 4:
		return "A"
	case 1, 5:
		return "B"
	case 2, 6:
		return "C"
	default:
		return "D"
	}
}

func getMSPs(endorsers []*Peer) map[string]struct{} {
	m := make(map[string]struct{})
	for _, endorser := range endorsers {
		m[getMSP(endorser)] = struct{}{}
	}
	return m
}

// testPeerKey is the ECDSA key shared by every fixture identity/envelope
// pair produced in this file. Tests here don't exercise per-org key
// separation (getMSP infers org from the endpoint string, not the key), so
// one shared key keeps peerIdentity/aliveMessage/stateInfoMessage
// independently callable, as they were before signature verification
// existed, while still producing envelopes that verify for real.
var testPeerKey = mustGenerateTestPeerKey()

func mustGenerateTestPeerKey() *ecdsa.PrivateKey {
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return sk
}

func signWithTestPeerKey(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, testPeerKey, digest[:])
	if err != nil {
		return nil, err
	}
	s, _, err = mspx509.ToLowS(&testPeerKey.PublicKey, s)
	if err != nil {
		return nil, err
	}
	return utils.MarshalECDSASignature(r, s)
}

func peerIdentity(mspID string, _ int) []byte {
	pubPEM, err := mspx509.PemEncodeKey(&testPeerKey.PublicKey)
	if err != nil {
		panic(err)
	}
	sID := &msp.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: pubPEM,
	}
	b, _ := proto.Marshal(sID)
	return b
}

func aliveMessage(id int) *gossip.Envelope {
	g := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: &gossip.AliveMessage{
				Timestamp: &gossip.PeerTime{
					SeqNum: uint64(id),
					IncNum: uint64(time.Now().UnixNano()),
				},
				Membership: &gossip.Member{
					Endpoint: fmt.Sprintf("p%d", id),
				},
			},
		},
	}
	sMsg, _ := signForTest(g)
	return sMsg.Envelope
}

func stateInfoMessage(chaincodes ...*gossip.Chaincode) *gossip.Envelope {
	return stateInfoMessageWithHeight(0, chaincodes...)
}

func stateInfoMessageWithHeight(ledgerHeight uint64, chaincodes ...*gossip.Chaincode) *gossip.Envelope {
	g := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{
				Timestamp: &gossip.PeerTime{
					SeqNum: 5,
					IncNum: uint64(time.Now().UnixNano()),
				},
				Properties: &gossip.Properties{
					Chaincodes:   chaincodes,
					LedgerHeight: ledgerHeight,
				},
			},
		},
	}
	sMsg, _ := signForTest(g)
	return sMsg.Envelope
}

type mockDiscoveryServer struct {
	mock.Mock
	*grpc.Server
	port int64
}

func newMockDiscoveryService() *mockDiscoveryServer {
	serverCert := loadFileOrPanic(filepath.Join("testdata", "server", "cert.pem"))
	serverKey := loadFileOrPanic(filepath.Join("testdata", "server", "key.pem"))
	srv, err := comm.NewGRPCServer("localhost:0", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:      true,
			Certificate: serverCert,
			Key:         serverKey,
		},
	})
	if err != nil {
		panic(err)
	}

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	s := srv.Server()
	d := &mockDiscoveryServer{
		Server: s,
	}
	discovery.RegisterDiscoveryServer(s, d)
	go func() {
		err := s.Serve(l)
		if errors.Is(err, grpc.ErrServerStopped) {
			return
		}
		if err != nil {
			panic(err)
		}
	}()
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	d.port, _ = strconv.ParseInt(portStr, 10, 64)
	return d
}

func (ds *mockDiscoveryServer) shutdown() {
	ds.Stop()
}

func (ds *mockDiscoveryServer) Discover(context.Context, *discovery.SignedRequest) (*discovery.Response, error) {
	args := ds.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*discovery.Response), nil
}

func ccCall(ccNames ...string) []*peer.ChaincodeCall {
	var call []*peer.ChaincodeCall
	for _, ccName := range ccNames {
		call = append(call, &peer.ChaincodeCall{
			Name: ccName,
		})
	}
	return call
}

func interest(ccNames ...string) *peer.ChaincodeInterest {
	interest := &peer.ChaincodeInterest{
		Chaincodes: []*peer.ChaincodeCall{},
	}
	for _, cc := range ccNames {
		interest.Chaincodes = append(interest.Chaincodes, &peer.ChaincodeCall{
			Name: cc,
		})
	}
	return interest
}

// signForTest creates a SignedGossipMessage signed with testPeerKey, so that
// it verifies against the identity produced by peerIdentity in tests that go
// through the signature-checked path (peersForChannel, endorser).
func signForTest(m *gossip.GossipMessage) (*SignedGossipMessage, error) {
	sMsg := &SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signWithTestPeerKey)
	return sMsg, err
}

func stateInfoWithHeight(h uint64) *SignedGossipMessage {
	g := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{
				Properties: &gossip.Properties{
					LedgerHeight: h,
				},
				Timestamp: &gossip.PeerTime{},
			},
		},
	}
	sMsg, _ := signForTest(g)
	return sMsg
}
