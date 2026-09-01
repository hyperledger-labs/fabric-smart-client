/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode/mock"
	discoveryApi "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// TestManager_Stop_StopsDiscoveryCache asserts a Manager's per-chaincode discovery
// cache goroutine stops when the Manager is stopped.
//
// The Manager is rooted at a context this test never cancels, so Stop() is the only
// thing that can retire the cache goroutine; goleak fails the test if it survives.
func TestManager_Stop_StopsDiscoveryCache(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	mockCS := &mock.ConfigService{}
	mockCS.NetworkNameReturns("test-network")
	mockCC := &mock.ChannelConfig{}
	mockCC.IDReturns("test-channel")
	mockCC.GetNumRetriesReturns(1)
	mockCC.GetRetrySleepReturns(0)
	mockCC.DiscoveryTimeoutReturns(time.Second)
	mockCC.DiscoveryDefaultTTLSReturns(time.Second)

	m := chaincode.NewManager(
		context.Background(),
		"test-network",
		"test-channel",
		mockCS,
		mockCC,
		1,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	// Force creation of a chaincode (and thus its discovery cache).
	require.NotNil(t, m.Chaincode("cc"))

	m.Stop()
}

// newPeerFixture returns a discovery peer carrying the minimum an alive
// message needs for toDiscoveredPeers to consider it, so that tests exercise
// validation rather than the earlier skip paths.
func newPeerFixture(t *testing.T, mspID, endpoint string, identity []byte) *discoveryApi.Peer {
	t.Helper()
	return &discoveryApi.Peer{
		MSPID:    mspID,
		Identity: identity,
		AliveMessage: &discoveryApi.SignedGossipMessage{
			GossipMessage: &gossip.GossipMessage{
				Content: &gossip.GossipMessage_AliveMsg{
					AliveMsg: &gossip.AliveMessage{
						Membership: &gossip.Member{Endpoint: endpoint},
						Timestamp:  &gossip.PeerTime{IncNum: 1, SeqNum: 1},
					},
				},
			},
		},
	}
}

// discoverPeers runs a peers-query discovery whose response reports the given
// peers, against a chaincode whose trust anchor is the fixture's MSPProvider.
func discoverPeers(t *testing.T, fix *discoveryTestFixture, peers ...*discoveryApi.Peer) ([]driver.DiscoveredPeer, error) {
	t.Helper()
	fix.ChannelResponse.PeersReturns(peers, nil)

	d := chaincode.NewDiscovery(fix.Chaincode)
	d.QueryForPeers = true
	return d.GetPeers()
}

func TestToDiscoveredPeersValidIdentityPasses(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	id := &mock.MSPIdentity{}
	id.GetMSPIdentifierReturns("Org1MSP")
	id.ValidateReturns(nil)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(id, nil)

	fix.MSPProvider.MSPManagerReturns(mgr)
	fix.MSPProvider.TLSRootCertsByMSPIDReturns([][]byte{[]byte("trusted-root")}, nil)

	peers, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("id-bytes")))
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "peer0:7051", peers[0].Endpoint)
	require.Equal(t, "Org1MSP", peers[0].MSPID)
	require.Equal(t, [][]byte{[]byte("trusted-root")}, peers[0].TLSRootCerts,
		"TLS roots must come from the channel config, not the discovery response")
}

func TestToDiscoveredPeersUntrustedCADropped(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(nil, errors.New("the supplied identity is not valid: x509: certificate signed by unknown authority"))

	fix.MSPProvider.MSPManagerReturns(mgr)

	peers, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "evil:7051", []byte("forged")))
	require.Error(t, err, "a response whose only peer fails validation must not succeed")
	require.Empty(t, peers)
	require.True(t, strings.Contains(err.Error(), "validation"),
		"error must attribute the empty result to validation, got [%v]", err)
}

func TestToDiscoveredPeersInvalidIdentityDropped(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	id := &mock.MSPIdentity{}
	id.GetMSPIdentifierReturns("Org1MSP")
	id.ValidateReturns(errors.New("certificate revoked"))

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(id, nil)

	fix.MSPProvider.MSPManagerReturns(mgr)

	peers, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "revoked:7051", []byte("id-bytes")))
	require.Error(t, err)
	require.Empty(t, peers)
}

func TestToDiscoveredPeersMSPIDMismatchDropped(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	// A genuine Org2 identity presented as an Org1 peer: valid in itself, but
	// not under the MSP the discovery response claimed.
	id := &mock.MSPIdentity{}
	id.GetMSPIdentifierReturns("Org2MSP")
	id.ValidateReturns(nil)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(id, nil)

	fix.MSPProvider.MSPManagerReturns(mgr)

	peers, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("org2-id")))
	require.Error(t, err)
	require.Empty(t, peers)
}

func TestToDiscoveredPeersRogueDroppedOthersKept(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	good := &mock.MSPIdentity{}
	good.GetMSPIdentifierReturns("Org1MSP")
	good.ValidateReturns(nil)

	mgr := &mock.MSPManager{}
	// First peer fails, second passes.
	mgr.DeserializeIdentityReturnsOnCall(0, nil, errors.New("certificate signed by unknown authority"))
	mgr.DeserializeIdentityReturnsOnCall(1, good, nil)

	fix.MSPProvider.MSPManagerReturns(mgr)
	fix.MSPProvider.TLSRootCertsByMSPIDReturns([][]byte{[]byte("trusted-root")}, nil)

	peers, err := discoverPeers(t, fix,
		newPeerFixture(t, "Org1MSP", "evil:7051", []byte("forged")),
		newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("genuine")),
	)
	require.NoError(t, err, "one rogue peer must not fail an otherwise satisfiable set")
	require.Len(t, peers, 1)
	require.Equal(t, "peer0:7051", peers[0].Endpoint)
}

func TestToDiscoveredPeersConfigRejectedFailsFast(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(nil, driver.ErrConfigRejected)

	fix.MSPProvider.MSPManagerReturns(mgr)

	start := time.Now()
	_, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("id-bytes")))
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second,
		"a rejected configuration must not be waited on")
}
