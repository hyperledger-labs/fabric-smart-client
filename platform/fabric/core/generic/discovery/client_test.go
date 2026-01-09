/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"context"
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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	comm "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	caCert := loadFileOrPanic(filepath.Join("testdata", "server", "ca.pem"))
	tlsConf := &tls.Config{
		RootCAs:      x509.NewCertPool(),
		Certificates: []tls.Certificate{certificate},
	}
	tlsConf.RootCAs.AppendCertsFromPEM(caCert)

	addr := fmt.Sprintf("localhost:%d", targetPort)
	return func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(addr, grpc.WithBlock(), grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
		require.NoError(t, err)
		if err != nil {
			panic(err)
		}
		return conn, nil
	}
}

func TestClient(t *testing.T) {
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

	//sup.On("PeersOfChannel").Return(channelPeersWithoutChaincodes).Times(2)
	req := NewRequest()
	req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	r, err := cl.Send(ctx, req, authInfo)
	require.NoError(t, err)

	t.Run("Channel mismatch", func(t *testing.T) {
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
		mychannel := r.ForChannel("mychannel")
		endorsers, err := mychannel.Endorsers(ccCall("mycc"), NoFilter)
		// However, since we didn't provide any chaincodes to these peers - the server shouldn't
		// be able to construct the descriptor.
		// Just check that the appropriate error is returned, and nothing crashes.
		require.Contains(t, err.Error(), "failed constructing descriptor for chaincode")
		require.Nil(t, endorsers)
	})

	t.Run("Endorser query with chaincodes installed", func(t *testing.T) {
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
			}}).Once()

		req = NewRequest()
		req.OfChannel("mychannel").AddPeersQuery().AddEndorsersQuery(interest("mycc"))
		r, err = cl.Send(ctx, req, authInfo)
		require.NoError(t, err)

		mychannel := r.ForChannel("mychannel")
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
	req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
	r, err := cl.Send(ctx, req, auth)
	require.Contains(t, err.Error(), "foo")
	require.Nil(t, r)

	// Scenario II: discovery service sends back an empty response
	svc.On("Discover").Return(&discovery.Response{}, nil).Once()
	req = NewRequest()
	req.OfChannel("mychannel").AddPeersQuery().AddConfigQuery().AddEndorsersQuery(interest("mycc"))
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
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
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
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
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
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
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
	req.OfChannel("mychannel").AddEndorsersQuery(interest("mycc"))
	r, err = cl.Send(ctx, req, auth)
	require.Contains(t, err.Error(), "group B isn't mapped to endorsers, but exists in a layout")
	require.Empty(t, r)
}

func TestAddEndorsersQueryInvalidInput(t *testing.T) {
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

func TestString(t *testing.T) {
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

func peerIdentity(mspID string, i int) []byte {
	p := fmt.Appendf(nil, "p%d", i)
	sID := &msp.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: p,
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
	sMsg, _ := noopSign(g)
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
	sMsg, _ := noopSign(g)
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
	go s.Serve(l)
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	d.port, _ = strconv.ParseInt(portStr, 10, 64)
	return d
}

func (ds *mockDiscoveryServer) shutdown() {
	ds.Server.Stop()
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

// noopSign creates a SignedGossipMessage with a nil signature
func noopSign(m *gossip.GossipMessage) (*SignedGossipMessage, error) {
	signer := func(msg []byte) ([]byte, error) {
		return nil, nil
	}
	sMsg := &SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signer)
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
	sMsg, _ := noopSign(g)
	return sMsg
}
