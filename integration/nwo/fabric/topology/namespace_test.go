/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
)

// twoOrgTopology has Org1 with one peer and Org2 with two, plus a channel.
// AddOrganization appends a new organization each call, so Org2's two peers
// must be chained off a single call.
func twoOrgTopology() *Topology {
	t := &Topology{
		Channels:    []*Channel{{Name: "testchannel", Default: true}},
		Consortiums: []*Consortium{{Name: "SampleConsortium"}},
	}
	t.AddOrganization("Org1").AddPeer("Org1_peer_0")
	t.AddOrganization("Org2").AddPeer("Org2_peer_0").AddPeer("Org2_peer_1")
	return t
}

func TestAddNamespaceDefaults(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("iou", Unanimity("Org1"))

	cc := ns.cc
	require.Equal(t, "iou", cc.Chaincode.Name)
	require.Equal(t, "iou", cc.Chaincode.Label)
	require.Equal(t, "Version-0.0", cc.Chaincode.Version)
	require.Equal(t, "1", cc.Chaincode.Sequence)
	require.Equal(t, "testchannel", cc.Channel)
	require.Equal(t, "AND ('Org1MSP.member')", cc.Chaincode.SignaturePolicy)

	require.Equal(t, BaseChaincodeImage, cc.Chaincode.Image)
	require.True(t, cc.Chaincode.IsCCaaS())
	require.Empty(t, cc.Chaincode.Path)

	require.Equal(t, `{"Args":["init"]}`, cc.Chaincode.Ctor)
	require.True(t, cc.Chaincode.InitRequired)

	require.Equal(t, []string{"Org1_peer_0"}, cc.Peers)
	require.Len(t, top.Chaincodes, 1, "the namespace must be registered")
}

func TestAddNamespacePeersComeFromPolicyOrgs(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("ns", Unanimity("Org2"))
	require.Equal(t, []string{"Org2_peer_0", "Org2_peer_1"}, ns.cc.Peers)
}

func TestAddNamespaceSignatureDefaultsToAllPeers(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("ns", Signature("OR ('Org1MSP.member')"))
	require.Equal(t,
		[]string{"Org1_peer_0", "Org2_peer_0", "Org2_peer_1"}, ns.cc.Peers)
}

func TestWithPeersOverridesDerivation(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("ns", Unanimity("Org1", "Org2"),
		WithPeers("Org2_peer_1"))
	require.Equal(t, []string{"Org2_peer_1"}, ns.cc.Peers)
}

func TestWithContainerImage(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("events", Signature("OR ('Org1MSP.member')"),
		WithContainerImage("fsc-cc/events:latest"))
	require.Equal(t, "fsc-cc/events:latest", ns.cc.Chaincode.Image)
	require.Empty(t, ns.cc.Chaincode.Path)
	require.Empty(t, ns.cc.Chaincode.Ctor, "a named image gets no default ctor")
	require.False(t, ns.cc.Chaincode.InitRequired)
}

func TestWithLegacyChaincode(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("asset_transfer", Unanimity("Org1"),
		WithLegacyChaincode(StateQueryChaincodePath))
	require.Equal(t, StateQueryChaincodePath, ns.cc.Chaincode.Path)
	require.Equal(t, "golang", ns.cc.Chaincode.Lang)
	require.Empty(t, ns.cc.Chaincode.Image)
	require.False(t, ns.cc.Chaincode.IsCCaaS())
}

func TestWithCtorEnablesInit(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("ns", Unanimity("Org1"),
		WithContainerImage("fsc-cc/events:latest"),
		WithCtor(`{"Args":["init","x"]}`))
	require.Equal(t, `{"Args":["init","x"]}`, ns.cc.Chaincode.Ctor)
	require.True(t, ns.cc.Chaincode.InitRequired)
}

// Options must commute: WithCtor before or after WithContainerImage is the
// same namespace either way.
func TestOptionsAreOrderIndependent(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	ctorFirst := twoOrgTopology().AddNamespace("ns", Unanimity("Org1"),
		WithCtor(`{"Args":["init"]}`),
		WithContainerImage("fsc-cc/events:latest"))
	imageFirst := twoOrgTopology().AddNamespace("ns", Unanimity("Org1"),
		WithContainerImage("fsc-cc/events:latest"),
		WithCtor(`{"Args":["init"]}`))

	require.Equal(t, *imageFirst.cc, *ctorFirst.cc)
	require.Equal(t, `{"Args":["init"]}`, ctorFirst.cc.Chaincode.Ctor)
	require.True(t, ctorFirst.cc.Chaincode.InitRequired)
}

func TestWithVersionAndPackageFile(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("ns", Unanimity("Org1"),
		WithLegacyChaincode("github.com/acme/cc"),
		WithVersion("Version-1.0"),
		WithPackageFile("/tmp/cc.tar.gz"))
	require.Equal(t, "Version-1.0", ns.cc.Chaincode.Version)
	require.Equal(t, "/tmp/cc.tar.gz", ns.cc.Chaincode.PackageFile)
}

func TestWithCtorSurvivesLegacyChaincode(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("asset_transfer", Unanimity("Org1"),
		WithLegacyChaincode(StateQueryChaincodePath),
		WithCtor(`{"Args":["init"]}`))
	require.Equal(t, `{"Args":["init"]}`, ns.cc.Chaincode.Ctor)
	require.True(t, ns.cc.Chaincode.InitRequired,
		"a legacy namespace with an explicit ctor must still require init")
}

// TestAddNamespaceSignatureRespectsChannelMembership covers peersForPolicy's
// channel filter: a peer joined to a different channel than the namespace
// must not be pulled in just because the policy names no organizations.
func TestAddNamespaceSignatureRespectsChannelMembership(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := &Topology{
		Channels:    []*Channel{{Name: "testchannel", Default: true}, {Name: "otherchannel"}},
		Consortiums: []*Consortium{{Name: "SampleConsortium"}},
	}
	top.AddOrganization("Org1").AddPeer("Org1_peer_0")
	top.AddOrganization("Org2").AddPeer("Org2_peer_0")

	// Org2_peer_0 is only joined to testchannel and otherchannel by default
	// (AddPeer joins every peer to every channel known at the time it is
	// added). To exercise the filter, strip Org2_peer_0 from testchannel so
	// it is only on otherchannel.
	for _, p := range top.Peers {
		if p.Name == "Org2_peer_0" {
			p.Channels = []*PeerChannel{{Name: "otherchannel", Anchor: true}}
		}
	}

	ns := top.AddNamespace("ns", Signature("OR ('Org1MSP.member')"))
	require.Equal(t, []string{"Org1_peer_0"}, ns.cc.Peers,
		"Org2_peer_0 is not on the namespace's channel and must be excluded")
}

// --- validation failure paths ---
//
// gomega.RegisterTestingT(t) wires Expect failures to t.Fatalf, which
// unwinds the calling goroutine with runtime.Goexit rather than panicking -
// so require.Panics can't observe it directly. recordValidationFailure runs
// the failing call on its own goroutine under a throwaway GomegaTestingT, so
// the Goexit only unwinds that goroutine, and reports whether it failed.

type recordingT struct {
	failed  bool
	message string
}

func (r *recordingT) Helper() {}

func (r *recordingT) Fatalf(format string, args ...any) {
	r.failed = true
	r.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// recordValidationFailure runs fn on its own goroutine with gomega directed
// at a recordingT, and restores t as gomega's target before returning.
func recordValidationFailure(t *testing.T, fn func()) (failed bool, message string) {
	t.Helper()
	rec := &recordingT{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		gomega.RegisterTestingT(rec)
		fn()
	}()
	<-done
	gomega.RegisterTestingT(t)
	return rec.failed, rec.message
}

func TestValidateNamespaceRejectsImageAndPath(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	failed, msg := recordValidationFailure(t, func() {
		top.AddNamespace("ns", Unanimity("Org1"), func(n *Namespace) {
			n.cc.Chaincode.Image = "fsc-cc/base:latest"
			n.cc.Chaincode.Path = "github.com/acme/cc"
		})
	})
	require.True(t, failed, "a namespace cannot be both a container image and a source path")
	require.Contains(t, msg, "cannot be both a container image and a source path")
}

func TestValidateNamespaceRejectsPackageFileWithContainerImage(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	failed, msg := recordValidationFailure(t, func() {
		top.AddNamespace("ns", Unanimity("Org1"),
			WithContainerImage("fsc-cc/events:latest"),
			WithPackageFile("/tmp/cc.tar.gz"))
	})
	require.True(t, failed, "WithPackageFile cannot be combined with a CCaaS container image")
	require.Contains(t, msg, "cannot combine WithPackageFile with WithContainerImage")
}

func TestValidateNamespaceRejectsEmptyPeerSet(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	failed, msg := recordValidationFailure(t, func() {
		top.AddNamespace("ns", Unanimity("Org1"), WithPeers())
	})
	require.True(t, failed, "a namespace resolving to no peers must be rejected")
	require.Contains(t, msg, "resolved to no peers")
}

func TestValidateNamespaceRejectsUnknownOrganization(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	failed, msg := recordValidationFailure(t, func() {
		top.AddNamespace("ns", Unanimity("Org1", "Orgx"))
	})
	require.True(t, failed, "Orgx is not declared in the topology and must be rejected")
	require.Contains(t, msg, "Orgx")
}

// --- migration-parity cases ---

// TestOneOutOfNPolicyMatchesLegacyShape covers OneOutOfN as the policy: the
// replacement for a legacy OR-of-orgs endorsement policy.
func TestOneOutOfNPolicyMatchesLegacyShape(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("ns", OneOutOfN("Org1", "Org2"))

	require.Equal(t, "OutOf (1, 'Org1MSP.member','Org2MSP.member')",
		ns.cc.Chaincode.SignaturePolicy)
	require.Equal(t, []string{"Org1_peer_0", "Org2_peer_0", "Org2_peer_1"}, ns.cc.Peers)
	require.Equal(t, "ns", ns.cc.Chaincode.Name)
	require.Equal(t, "testchannel", ns.cc.Channel)
}

// TestSignatureContainerImageAndPeersMatchAddManagedNamespaceShape covers the
// retired AddManagedNamespace's shape: a verbatim Signature rule, a container
// image, and explicit peers, as used by integration/fabric/atsachaincode.
func TestSignatureContainerImageAndPeersMatchAddManagedNamespaceShape(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	top := twoOrgTopology()
	ns := top.AddNamespace("asset_transfer",
		Signature(`OR ('Org1MSP.member','Org2MSP.member')`),
		WithContainerImage("fsc-cc/atsachaincode:latest"),
		WithPeers("Org1_peer_0", "Org2_peer_0"))

	require.Equal(t, `OR ('Org1MSP.member','Org2MSP.member')`, ns.cc.Chaincode.SignaturePolicy)
	require.Equal(t, "fsc-cc/atsachaincode:latest", ns.cc.Chaincode.Image)
	require.True(t, ns.cc.Chaincode.IsCCaaS())
	require.Equal(t, []string{"Org1_peer_0", "Org2_peer_0"}, ns.cc.Peers)
	require.Equal(t, "asset_transfer", ns.cc.Chaincode.Name)
}
