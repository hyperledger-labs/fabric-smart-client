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

// TestToDiscoveredPeersConfigRejectedPropagates asserts that a rejected
// channel configuration is reported as a configuration problem, not as a
// peer that failed MSP validation. Those are different operational
// conditions: without this distinction, a node starting up during a config
// race reports its own peers as cryptographically untrusted, sending an
// operator hunting a CA problem that does not exist.
func TestToDiscoveredPeersConfigRejectedPropagates(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(nil, driver.ErrConfigRejected)

	fix.MSPProvider.MSPManagerReturns(mgr)

	_, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("id-bytes")))
	require.ErrorIs(t, err, driver.ErrConfigRejected)
	require.NotContains(t, err.Error(), "failed MSP validation",
		"a configuration that is unavailable must not be reported as peers failing validation")
}

// TestToDiscoveredPeersNotInitializedPropagates is
// TestToDiscoveredPeersConfigRejectedPropagates's twin for the other
// deferred-configuration error: deleting just the ErrNotInitialized disjunct
// from toDiscoveredPeers's fail-fast check leaves the ErrConfigRejected test
// green, so both errors need their own witness.
func TestToDiscoveredPeersNotInitializedPropagates(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(nil, driver.ErrNotInitialized)

	fix.MSPProvider.MSPManagerReturns(mgr)

	_, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("id-bytes")))
	require.ErrorIs(t, err, driver.ErrNotInitialized)
	require.NotContains(t, err.Error(), "failed MSP validation",
		"a configuration that is unavailable must not be reported as peers failing validation")
}

// TestToDiscoveredPeersEmptySetIsNotAValidationFailure asserts the design
// property that a legitimately-empty filter result (no peers were ever
// discovered, so none were rejected) is not itself reported as a validation
// failure - only rejected > 0 warrants that error.
func TestToDiscoveredPeersEmptySetIsNotAValidationFailure(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)
	peers, err := discoverPeers(t, fix)
	require.NoError(t, err, "an empty discovery result is not a validation failure")
	require.Empty(t, peers)
}

// TestToDiscoveredPeersTimedOutWaitPropagatesAsNotInitialized is
// TestToDiscoveredPeersNotInitializedPropagates's twin for a real waiting
// accessor's timeout rather than a mock returning the sentinel directly: it
// asserts that a wait that ran out - reported as ErrNotLoaded wrapped with a
// "timed out waiting" message, exactly what deferred.Holder.WaitForValue now
// produces - is still classified as a configuration problem and not
// misreported as every peer having failed MSP validation.
func TestToDiscoveredPeersTimedOutWaitPropagatesAsNotInitialized(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(nil, errors.Wrapf(driver.ErrNotInitialized,
		"channel [test-channel] configuration not loaded: timed out waiting for channel [test-channel] configuration: context deadline exceeded"))

	fix.MSPProvider.MSPManagerReturns(mgr)

	_, err := discoverPeers(t, fix, newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("id-bytes")))
	require.ErrorIs(t, err, driver.ErrNotInitialized)
	require.NotContains(t, err.Error(), "failed MSP validation",
		"a wait that timed out must not be reported as peers failing validation")
}

// TestToDiscoveredPeersResolvesMSPManagerOncePerCall asserts that
// toDiscoveredPeers obtains the driver.MSPManager once for the whole batch of
// peers rather than once per peer. Without this, a waiting MSPManager pays its
// full wait budget again for every peer instead of once per call: with N
// peers that is N times the documented bound. Three peers whose validation
// all fails on the same config-unavailable path (the fail-fast branch returns
// on the first one) must still only cost a single MSPManager() call.
func TestToDiscoveredPeersResolvesMSPManagerOncePerCall(t *testing.T) {
	t.Parallel()

	fix := setupDiscoveryTest(t)

	mgr := &mock.MSPManager{}
	mgr.DeserializeIdentityReturns(nil, driver.ErrNotInitialized)

	fix.MSPProvider.MSPManagerReturns(mgr)

	_, err := discoverPeers(t, fix,
		newPeerFixture(t, "Org1MSP", "peer0:7051", []byte("id-bytes-0")),
		newPeerFixture(t, "Org1MSP", "peer1:7051", []byte("id-bytes-1")),
		newPeerFixture(t, "Org1MSP", "peer2:7051", []byte("id-bytes-2")),
	)
	require.ErrorIs(t, err, driver.ErrNotInitialized)
	require.Equal(t, 1, fix.MSPProvider.MSPManagerCallCount(),
		"MSPManager must be resolved once per toDiscoveredPeers call, not once per peer")
}
