/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"context"
	"crypto/tls"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	discovery2 "github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode/mock"
	discoveryApi "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fscGrpc "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/signing_identity.go -fake-name SigningIdentity github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.SigningIdentity
//go:generate counterfeiter -o mock/config_service.go -fake-name ConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ConfigService
//go:generate counterfeiter -o mock/channel_config.go -fake-name ChannelConfig github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChannelConfig
//go:generate counterfeiter -o mock/local_membership.go -fake-name LocalMembership github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.LocalMembership
//go:generate counterfeiter -o mock/signer_service.go -fake-name SignerService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.SignerService
//go:generate counterfeiter -o mock/finality.go -fake-name Finality github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Finality
//go:generate counterfeiter -o mock/services.go -fake-name Services github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode.Services
//go:generate counterfeiter -o mock/broadcaster.go -fake-name Broadcaster github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode.Broadcaster
//go:generate counterfeiter -o mock/msp_provider.go -fake-name MSPProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode.MSPProvider
//go:generate counterfeiter -o mock/discovery_client.go -fake-name DiscoveryClient github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery.DiscoveryClient
//go:generate counterfeiter -o mock/channel_response.go -fake-name ChannelResponse github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery.ChannelResponse
//go:generate counterfeiter -o mock/endorser_client.go -fake-name EndorserClient github.com/hyperledger/fabric-protos-go-apiv2/peer.EndorserClient
//go:generate counterfeiter -o mock/peer_client.go -fake-name PeerClient github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services.PeerClient
//go:generate counterfeiter -o mock/response.go -fake-name Response github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery.Response
//go:generate counterfeiter -o mock/chaincode_config.go -fake-name ChaincodeConfig github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChaincodeConfig
//go:generate counterfeiter -o mock/msp_manager.go -fake-name MSPManager github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.MSPManager

type mockMSPIdentity struct {
	mspID string
}

func (m *mockMSPIdentity) GetMSPIdentifier() string           { return m.mspID }
func (m *mockMSPIdentity) Validate() error                    { return nil }
func (m *mockMSPIdentity) Verify(message, sigma []byte) error { return nil }

type chaincodeTestFixture struct {
	Chaincode       *chaincode.Chaincode
	ConfigService   *mock.ConfigService
	ChannelConfig   *mock.ChannelConfig
	LocalMembership *mock.LocalMembership
	Services        *mock.Services
	SignerService   *mock.SignerService
	Broadcaster     *mock.Broadcaster
	Finality        *mock.Finality
	MSPProvider     *mock.MSPProvider
}

type discoveryTestFixture struct {
	*chaincodeTestFixture
	DiscoveryClient *mock.DiscoveryClient
	ChannelResponse *mock.ChannelResponse
	Peer            *discoveryApi.Peer
}

type invokeTestFixture struct {
	*chaincodeTestFixture
	EndorserClient *mock.EndorserClient
	PrpBytes       []byte
}

func setupTestChaincode(t *testing.T) *chaincodeTestFixture {
	t.Helper()
	mockCS := &mock.ConfigService{}
	mockCC := &mock.ChannelConfig{}
	mockLM := &mock.LocalMembership{}
	mockServices := &mock.Services{}
	mockSS := &mock.SignerService{}
	mockB := &mock.Broadcaster{}
	mockF := &mock.Finality{}
	mockMSPProv := &mock.MSPProvider{}

	// Setup some defaults
	mockCS.NetworkNameReturns("test-network")
	mockCC.IDReturns("test-channel")
	mockCC.GetNumRetriesReturns(1)
	mockCC.GetRetrySleepReturns(0)
	mockCC.DiscoveryTimeoutReturns(time.Second)
	mockCC.DiscoveryDefaultTTLSReturns(time.Second)

	c := chaincode.NewChaincode(
		"test-chaincode",
		mockCS,
		mockCC,
		mockLM,
		mockServices,
		mockSS,
		mockB,
		mockF,
		mockMSPProv,
	)

	return &chaincodeTestFixture{
		Chaincode:       c,
		ConfigService:   mockCS,
		ChannelConfig:   mockCC,
		LocalMembership: mockLM,
		Services:        mockServices,
		SignerService:   mockSS,
		Broadcaster:     mockB,
		Finality:        mockF,
		MSPProvider:     mockMSPProv,
	}
}

func createValidProposalResponsePayload(t *testing.T) []byte {
	t.Helper()
	cca := &pb.ChaincodeAction{
		Results: []byte("my-read-write-set"),
	}
	ccaBytes, err := proto.Marshal(cca)
	require.NoError(t, err)

	prp := &pb.ProposalResponsePayload{
		Extension: ccaBytes,
	}
	prpBytes, err := proto.Marshal(prp)
	require.NoError(t, err)

	return prpBytes
}

func setupDiscoveryTest(t *testing.T) *discoveryTestFixture {
	t.Helper()
	fix := setupTestChaincode(t)

	fix.ConfigService.PickPeerReturns(&fscGrpc.ConnectionConfig{Address: "localhost:7051"})

	mockPC := &mock.PeerClient{}
	mockPC.AddressReturns("localhost:7051")
	mockPC.CertificateReturns(tls.Certificate{
		Certificate: [][]byte{[]byte("fake-cert")},
	})
	fix.Services.NewPeerClientReturns(mockPC, nil)

	mockSigner := &mock.SigningIdentity{}
	mockSigner.SerializeReturns([]byte("mock-creator"), nil)
	fix.LocalMembership.DefaultSigningIdentityReturns(mockSigner)

	mockDC := &mock.DiscoveryClient{}
	mockPC.DiscoveryClientReturns(mockDC, nil)

	mockResp := &mock.Response{}
	mockChanResp := &mock.ChannelResponse{}
	mockResp.ForChannelReturns(mockChanResp)
	mockDC.SendReturns(mockResp, nil)

	peer1 := &discoveryApi.Peer{
		MSPID:    "Org1MSP",
		Identity: []byte("peer1-identity"),
		AliveMessage: &discoveryApi.SignedGossipMessage{
			GossipMessage: &gossip.GossipMessage{
				Content: &gossip.GossipMessage_AliveMsg{
					AliveMsg: &gossip.AliveMessage{
						Membership: &gossip.Member{
							Endpoint: "peer1.org1.example.com:7051",
						},
					},
				},
			},
		},
	}

	configResult := &discovery2.ConfigResult{
		Msps: map[string]*msp.FabricMSPConfig{
			"Org1MSP": {
				TlsRootCerts: [][]byte{[]byte("root-cert-1")},
			},
		},
	}
	mockChanResp.ConfigReturns(configResult, nil)

	return &discoveryTestFixture{
		chaincodeTestFixture: fix,
		DiscoveryClient:      mockDC,
		ChannelResponse:      mockChanResp,
		Peer:                 peer1,
	}
}

func setupInvokeTest(t *testing.T) *invokeTestFixture {
	t.Helper()
	fix := setupTestChaincode(t)

	fix.ConfigService.PickPeerReturns(&fscGrpc.ConnectionConfig{Address: "localhost:7051"})

	mockPC := &mock.PeerClient{}
	mockPC.AddressReturns("localhost:7051")
	fix.Services.NewPeerClientReturns(mockPC, nil)

	mockSigner := &mock.SigningIdentity{}
	mockSigner.SerializeReturns([]byte("mock-creator"), nil)
	mockSigner.SignReturns([]byte("mock-signature"), nil)
	fix.LocalMembership.DefaultSigningIdentityReturns(mockSigner)
	fix.SignerService.GetSigningIdentityReturns(mockSigner, nil)

	mockEndorserClient := &mock.EndorserClient{}
	mockPC.EndorserClientReturns(mockEndorserClient, nil)

	// Mock discovery
	mockDC := &mock.DiscoveryClient{}
	mockPC.DiscoveryClientReturns(mockDC, nil)
	mockResp := &mock.Response{}
	mockChanResp := &mock.ChannelResponse{}
	mockResp.ForChannelReturns(mockChanResp)
	mockDC.SendReturns(mockResp, nil)

	peer1 := &discoveryApi.Peer{
		MSPID:    "Org1MSP",
		Identity: []byte("peer1-identity"),
		AliveMessage: &discoveryApi.SignedGossipMessage{
			GossipMessage: &gossip.GossipMessage{
				Content: &gossip.GossipMessage_AliveMsg{
					AliveMsg: &gossip.AliveMessage{
						Membership: &gossip.Member{Endpoint: "localhost:7051"},
					},
				},
			},
		},
	}
	mockChanResp.EndorsersReturns([]*discoveryApi.Peer{peer1}, nil)

	configResult := &discovery2.ConfigResult{
		Msps: map[string]*msp.FabricMSPConfig{
			"Org1MSP": {
				TlsRootCerts: [][]byte{[]byte("root-cert-1")},
			},
		},
	}
	mockChanResp.ConfigReturns(configResult, nil)

	prpBytes := createValidProposalResponsePayload(t)

	mockEndorserClient.ProcessProposalReturns(&pb.ProposalResponse{
		Response: &pb.Response{
			Status:  200,
			Payload: []byte("success-payload"),
		},
		Payload: prpBytes,
		Endorsement: &pb.Endorsement{
			Endorser: []byte("peer1-identity"),
		},
	}, nil)

	return &invokeTestFixture{
		chaincodeTestFixture: fix,
		EndorserClient:       mockEndorserClient,
		PrpBytes:             prpBytes,
	}
}

func TestNewChaincode(t *testing.T) {
	t.Parallel()
	fix := setupTestChaincode(t)
	require.NotNil(t, fix.Chaincode)
	require.Equal(t, "test-network", fix.Chaincode.NetworkID)
	require.Equal(t, "test-channel", fix.Chaincode.ChannelID)

	inv := fix.Chaincode.NewInvocation("my-func", "arg1")
	require.NotNil(t, inv)

	disc := fix.Chaincode.NewDiscover()
	require.NotNil(t, disc)
}

func TestChaincodeIsPrivate(t *testing.T) {
	t.Parallel()
	t.Run("channel nil", func(t *testing.T) {
		t.Parallel()
		fix := setupTestChaincode(t)
		fix.ConfigService.ChannelReturns(nil)
		require.False(t, fix.Chaincode.IsPrivate())
	})

	t.Run("chaincode not found", func(t *testing.T) {
		t.Parallel()
		fix := setupTestChaincode(t)
		mockChan := &mock.ChannelConfig{}
		mockChan.ChaincodeConfigsReturns(nil) // empty
		fix.ConfigService.ChannelReturns(mockChan)
		require.False(t, fix.Chaincode.IsPrivate())
	})

	t.Run("chaincode found", func(t *testing.T) {
		t.Parallel()
		fix := setupTestChaincode(t)
		mockChan := &mock.ChannelConfig{}
		mockCCConf := &mock.ChaincodeConfig{}
		mockCCConf.IDReturns("test-chaincode")
		mockCCConf.IsPrivateReturns(true)
		mockChan.ChaincodeConfigsReturns([]driver.ChaincodeConfig{mockCCConf})
		fix.ConfigService.ChannelReturns(mockChan)
		require.True(t, fix.Chaincode.IsPrivate())
	})
}

func TestManager(t *testing.T) {
	t.Parallel()
	fix := setupTestChaincode(t)

	mgr := chaincode.NewManager(
		"test-network",
		"test-channel",
		fix.ConfigService,
		fix.ChannelConfig,
		1,
		0,
		fix.LocalMembership,
		fix.Services,
		fix.SignerService,
		fix.Broadcaster,
		fix.Finality,
		fix.MSPProvider,
	)

	require.NotNil(t, mgr)

	cc1 := mgr.Chaincode("cc1")
	require.NotNil(t, cc1)
	cc2 := mgr.Chaincode("cc1")
	require.Equal(t, cc1, cc2) // must be cached

	cc3 := mgr.Chaincode("cc2")
	require.NotEqual(t, cc1, cc3)
}

func TestDiscovery_NewDiscovery(t *testing.T) {
	t.Parallel()
	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)

		_, err := d.GetEndorsers()
		require.NoError(t, err)
		require.Equal(t, 1, fix.DiscoveryClient.SendCallCount())

		_, err = d.GetEndorsers()
		require.NoError(t, err)
		require.Equal(t, 1, fix.DiscoveryClient.SendCallCount())
	})

	t.Run("from chaincode channel config", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelConfig.DiscoveryTimeoutReturns(2 * time.Second)
		fix.ChannelConfig.DiscoveryDefaultTTLSReturns(10 * time.Millisecond)
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)

		_, err := d.GetEndorsers()
		require.NoError(t, err)
		require.Equal(t, 1, fix.DiscoveryClient.SendCallCount())

		_, err = d.GetEndorsers()
		require.NoError(t, err)
		require.Equal(t, 1, fix.DiscoveryClient.SendCallCount())

		time.Sleep(50 * time.Millisecond)

		_, err = d.GetEndorsers()
		require.NoError(t, err)
		require.Equal(t, 2, fix.DiscoveryClient.SendCallCount())
	})
}

func TestDiscovery_CallAndGetEndorsers(t *testing.T) {
	t.Parallel()

	t.Run("GetEndorsers Success", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		d.WithFilterByMSPIDs("Org1MSP")

		peers, err := d.Call()
		require.NoError(t, err)
		require.Len(t, peers, 1)
		require.Equal(t, "peer1.org1.example.com:7051", peers[0].Endpoint)
		require.Equal(t, "Org1MSP", peers[0].MSPID)
		require.Equal(t, [][]byte{[]byte("root-cert-1")}, peers[0].TLSRootCerts)
	})

	t.Run("GetEndorsers Success with Implicit Collections", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		d.WithImplicitCollections("Org1MSP")

		peers, err := d.Call()
		require.NoError(t, err)
		require.Len(t, peers, 1)
	})

	t.Run("GetPeers Success", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.PeersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		d.WithForQuery()
		d.WithFilterByMSPIDs("Org1MSP")

		peers, err := d.Call()
		require.NoError(t, err)
		require.Len(t, peers, 1)
	})

	t.Run("GetPeers Success with Implicit Collections", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.PeersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		d.WithForQuery()
		d.WithImplicitCollections("Org1MSP")

		peers, err := d.Call()
		require.NoError(t, err)
		require.Len(t, peers, 1)
	})

	t.Run("Discovery response error", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.DiscoveryClient.SendReturns(nil, errors.New("send-error"))
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.Call()
		require.ErrorContains(t, err, "failed to get discovery response")
	})

	t.Run("Endorsers error", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns(nil, errors.New("endorsers-error"))
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.Call()
		require.ErrorContains(t, err, "failed getting endorsers")
	})

	t.Run("Config error", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{fix.Peer}, nil)
		fix.ChannelResponse.ConfigReturns(nil, errors.New("config-error"))
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.Call()
		require.ErrorContains(t, err, "failed getting config")
	})
}

func TestDiscovery_ChaincodeVersion(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerWithVersion := &discoveryApi.Peer{
			MSPID:    "Org1MSP",
			Identity: []byte("peer1-identity"),
			StateInfoMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_StateInfo{
						StateInfo: &gossip.StateInfo{
							Properties: &gossip.Properties{
								Chaincodes: []*gossip.Chaincode{
									{
										Name:    "test-chaincode",
										Version: "1.0",
									},
								},
							},
						},
					},
				},
			},
		}

		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerWithVersion}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		version, err := d.ChaincodeVersion()
		require.NoError(t, err)
		require.Equal(t, "1.0", version)
	})

	t.Run("Discovery response error", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.DiscoveryClient.SendReturns(nil, errors.New("send-error"))
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.Error(t, err)
	})

	t.Run("Endorsers error", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns(nil, errors.New("endorsers-error"))
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.Error(t, err)
	})

	t.Run("No endorsers", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.ErrorContains(t, err, "no endorsers found")
	})

	t.Run("StateInfoMessage is nil", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerNoStateInfo := &discoveryApi.Peer{
			MSPID:            "Org1MSP",
			Identity:         []byte("peer1-identity"),
			StateInfoMessage: nil,
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerNoStateInfo}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.ErrorContains(t, err, "no state info message found")
	})

	t.Run("StateInfo is nil", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerNilStateInfo := &discoveryApi.Peer{
			MSPID:    "Org1MSP",
			Identity: []byte("peer1-identity"),
			StateInfoMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_StateInfo{
						StateInfo: nil,
					},
				},
			},
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerNilStateInfo}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.ErrorContains(t, err, "no state info found")
	})

	t.Run("Properties is nil", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerNilProperties := &discoveryApi.Peer{
			MSPID:    "Org1MSP",
			Identity: []byte("peer1-identity"),
			StateInfoMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_StateInfo{
						StateInfo: &gossip.StateInfo{
							Properties: nil,
						},
					},
				},
			},
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerNilProperties}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.ErrorContains(t, err, "no properties found")
	})

	t.Run("No chaincode info", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerNoCC := &discoveryApi.Peer{
			MSPID:    "Org1MSP",
			Identity: []byte("peer1-identity"),
			StateInfoMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_StateInfo{
						StateInfo: &gossip.StateInfo{
							Properties: &gossip.Properties{
								Chaincodes: []*gossip.Chaincode{},
							},
						},
					},
				},
			},
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerNoCC}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.ErrorContains(t, err, "no chaincode info found")
	})

	t.Run("Chaincode mismatch", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerMismatchCC := &discoveryApi.Peer{
			MSPID:    "Org1MSP",
			Identity: []byte("peer1-identity"),
			StateInfoMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_StateInfo{
						StateInfo: &gossip.StateInfo{
							Properties: &gossip.Properties{
								Chaincodes: []*gossip.Chaincode{
									{
										Name:    "different-chaincode",
										Version: "1.0",
									},
								},
							},
						},
					},
				},
			},
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerMismatchCC}, nil)
		d := chaincode.NewDiscovery(fix.Chaincode)
		_, err := d.ChaincodeVersion()
		require.ErrorContains(t, err, "not found")
	})
}

func TestChaincodeIsAvailableAndVersion(t *testing.T) {
	t.Parallel()

	t.Run("IsAvailable Success", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peer1 := &discoveryApi.Peer{
			MSPID: "Org1MSP",
			AliveMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_AliveMsg{
						AliveMsg: &gossip.AliveMessage{
							Membership: &gossip.Member{Endpoint: "peer1:7051"},
						},
					},
				},
			},
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peer1}, nil)
		avail, err := fix.Chaincode.IsAvailable()
		require.NoError(t, err)
		require.True(t, avail)
	})

	t.Run("IsAvailable Fail", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		fix.ChannelResponse.EndorsersReturns(nil, errors.New("discovery-failed"))
		avail, err := fix.Chaincode.IsAvailable()
		require.Error(t, err)
		require.False(t, avail)
	})

	t.Run("Version Success", func(t *testing.T) {
		t.Parallel()
		fix := setupDiscoveryTest(t)
		peerWithVersion := &discoveryApi.Peer{
			MSPID:    "Org1MSP",
			Identity: []byte("peer1-identity"),
			StateInfoMessage: &discoveryApi.SignedGossipMessage{
				GossipMessage: &gossip.GossipMessage{
					Content: &gossip.GossipMessage_StateInfo{
						StateInfo: &gossip.StateInfo{
							Properties: &gossip.Properties{
								Chaincodes: []*gossip.Chaincode{
									{
										Name:    "test-chaincode",
										Version: "2.0",
									},
								},
							},
						},
					},
				},
			},
		}
		fix.ChannelResponse.EndorsersReturns([]*discoveryApi.Peer{peerWithVersion}, nil)
		version, err := fix.Chaincode.Version()
		require.NoError(t, err)
		require.Equal(t, "2.0", version)
	})
}

func TestInvoke_WithFluentMethods(t *testing.T) {
	t.Parallel()
	fix := setupTestChaincode(t)
	inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")

	inv = inv.WithContext(t.Context()).(*chaincode.Invoke)
	require.Equal(t, t.Context(), inv.Ctx)

	inv = inv.WithEndorsersByMSPIDs("Org1MSP").(*chaincode.Invoke)
	require.Equal(t, []string{"Org1MSP"}, inv.EndorsersMSPIDs)

	inv = inv.WithEndorsersFromMyOrg().(*chaincode.Invoke)
	require.True(t, inv.EndorsersFromMyOrg)

	inv = inv.WithDiscoveredEndorsersByEndpoints("peer1").(*chaincode.Invoke)
	require.Equal(t, []string{"peer1"}, inv.DiscoveredEndorsersByEndpoints)

	inv = inv.WithMatchEndorsementPolicy().(*chaincode.Invoke)
	require.True(t, inv.MatchEndorsementPolicy)

	inv = inv.WithSignerIdentity(view.Identity("my-identity")).(*chaincode.Invoke)
	require.Equal(t, view.Identity("my-identity"), inv.SignerIdentity)

	connConfig := &fscGrpc.ConnectionConfig{Address: "peer1:7051"}
	inv = inv.WithEndorsersByConnConfig(connConfig).(*chaincode.Invoke)
	require.Equal(t, []*fscGrpc.ConnectionConfig{connConfig}, inv.EndorsersByConnConfig)

	inv = inv.WithImplicitCollections("coll1").(*chaincode.Invoke)
	require.Equal(t, []string{"coll1"}, inv.ImplicitCollectionMSPIDs)

	txID := driver.TxIDComponents{Nonce: []byte("nonce1")}
	inv = inv.WithTxID(txID).(*chaincode.Invoke)
	require.Equal(t, txID, inv.TxID)

	inv = inv.WithNumRetries(3).(*chaincode.Invoke)
	require.Equal(t, 3, inv.NumRetries)

	inv = inv.WithRetrySleep(5 * time.Millisecond).(*chaincode.Invoke)
	require.Equal(t, 5*time.Millisecond, inv.RetrySleep)

	// Test toBytes coverage
	var err error
	_, err = inv.WithTransientEntry("k1", []byte("val1"))
	require.NoError(t, err)
	require.Equal(t, []byte("val1"), inv.TransientMap["k1"])

	_, err = inv.WithTransientEntry("k2", "val2")
	require.NoError(t, err)
	require.Equal(t, []byte("val2"), inv.TransientMap["k2"])

	_, err = inv.WithTransientEntry("k3", int(10))
	require.NoError(t, err)
	require.Equal(t, []byte("10"), inv.TransientMap["k3"])

	_, err = inv.WithTransientEntry("k4", int64(20))
	require.NoError(t, err)
	require.Equal(t, []byte("20"), inv.TransientMap["k4"])

	_, err = inv.WithTransientEntry("k5", uint64(30))
	require.NoError(t, err)
	require.Equal(t, []byte("30"), inv.TransientMap["k5"])

	_, err = inv.WithTransientEntry("k6", true) // unsupported
	require.Error(t, err)
}

func TestInvoke_EndorseQuerySubmit(t *testing.T) {
	t.Parallel()

	t.Run("Endorse Success via ConnConfig", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		inv.WithSignerIdentity(view.Identity("user1"))
		inv.WithEndorsersByConnConfig(&fscGrpc.ConnectionConfig{Address: "localhost:7051"})

		env, err := inv.Endorse()
		require.NoError(t, err)
		require.NotNil(t, env)
	})

	t.Run("Endorse Success via Discovery", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		inv.WithSignerIdentity(view.Identity("user1"))
		inv.WithEndorsersFromMyOrg()

		mockMSPManager := &mock.MSPManager{}
		mockMSPManager.DeserializeIdentityReturns(&mockMSPIdentity{mspID: "Org1MSP"}, nil)
		fix.MSPProvider.MSPManagerReturns(mockMSPManager)

		env, err := inv.Endorse()
		require.NoError(t, err)
		require.NotNil(t, env)
	})

	t.Run("Endorse Fail on Prepare (Signer error)", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		// no signer specified, should fail
		env, err := inv.Endorse()
		require.Error(t, err)
		require.Nil(t, env)
	})

	t.Run("Query Success", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		inv.WithSignerIdentity(view.Identity("user1"))
		inv.WithEndorsersByConnConfig(&fscGrpc.ConnectionConfig{Address: "localhost:7051"})

		res, err := inv.Query()
		require.NoError(t, err)
		require.Equal(t, []byte("success-payload"), res)
	})

	t.Run("Query Failure (response mismatch)", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		inv.WithSignerIdentity(view.Identity("user1"))
		inv.WithEndorsersByConnConfig(
			&fscGrpc.ConnectionConfig{Address: "localhost:7051"},
			&fscGrpc.ConnectionConfig{Address: "localhost:7052"},
		)

		var callCount int32
		fix.EndorserClient.ProcessProposalCalls(func(ctx context.Context, prop *pb.SignedProposal, opts ...grpc.CallOption) (*pb.ProposalResponse, error) {
			payload := "payload1"
			if atomic.AddInt32(&callCount, 1) > 1 {
				payload = "payload2"
			}
			return &pb.ProposalResponse{
				Response: &pb.Response{
					Status:  200,
					Payload: []byte(payload),
				},
				Payload: fix.PrpBytes,
				Endorsement: &pb.Endorsement{
					Endorser: []byte("peer-identity"),
				},
			}, nil
		})

		res, err := inv.Query()
		require.ErrorContains(t, err, "payload does not match")
		require.Nil(t, res)
	})

	t.Run("Submit Success", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		inv.WithSignerIdentity(view.Identity("user1"))
		inv.WithEndorsersByConnConfig(&fscGrpc.ConnectionConfig{Address: "localhost:7051"})

		fix.Broadcaster.BroadcastReturns(nil)
		fix.Finality.IsFinalReturns(nil)

		txID, res, err := inv.Submit()
		require.NoError(t, err)
		require.NotEmpty(t, txID)
		require.Equal(t, []byte("success-payload"), res)
	})

	t.Run("Submit Fail on Broadcast", func(t *testing.T) {
		t.Parallel()
		fix := setupInvokeTest(t)
		inv := chaincode.NewInvoke(fix.Chaincode, "my-func", "arg1")
		inv.WithSignerIdentity(view.Identity("user1"))
		inv.WithEndorsersByConnConfig(&fscGrpc.ConnectionConfig{Address: "localhost:7051"})

		fix.Broadcaster.BroadcastReturns(errors.New("broadcast-failed"))

		txID, res, err := inv.Submit()
		require.ErrorContains(t, err, "broadcast-failed")
		require.Empty(t, txID)
		require.Nil(t, res)
	})
}
