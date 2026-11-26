/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlog_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rekorv1 "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const viewTimeout = 2 * time.Minute

// nodes are the three log nodes, in Org1..Org3 order.
var nodes = []string{"org1", "org2", "org3"}

var _ = Describe("EndToEnd", func() {
	Describe("Transparency log", Ordered, Label(nwofsc.WebSocket), func() {
		s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
			ii, err := integration.New(integration.FabricXTlogPort.StartPortForNode(), "", tlog.Topology(&tlog.SDK{}, nwofsc.WebSocket)...)
			if err != nil {
				return nil, err
			}
			ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
			ii.Generate()
			return ii, nil
		})
		// One network for all specs: the log carries over from one to the next.
		BeforeAll(s.Setup)
		AfterAll(s.TearDown)

		// alice (through org1), bob (through org2) and carol (through org3)
		// are the demo's "fake cosign" clients: they never trust the node they talk to, verifying
		// everything it hands back against all three orgs' keys instead.
		var verifiers []note.Verifier
		var alice, bob, carol *client.Client
		BeforeEach(func() {
			verifiers = initNodes(s.II)
			alice = newClient(s.II, "org1", verifiers)
			bob = newClient(s.II, "org2", verifiers)
			carol = newClient(s.II, "org3", verifiers)
		})

		It("logs an entry that all three orgs endorsed and signed", func() {
			ctx, cancel := viewContext()
			defer cancel()

			entry, err := alice.Log(ctx, []byte("artifact 0"))
			Expect(err).NotTo(HaveOccurred())
			Expect(entry.LogIndex).To(BeEquivalentTo(0))
		})

		It("rejects an entry with a bad signature", func() {
			ctx, cancel := viewContext()
			defer cancel()

			// alice.Log only ever signs honestly; a bad signature has to be
			// crafted and submitted directly, below the client.
			_, err := logArtifact(s.II, "org1", []byte("artifact 1"), []byte("another artifact"))
			Expect(err).To(HaveOccurred())

			// The first spec logged one entry; the rejected one took no index.
			entry, err := alice.Log(ctx, []byte("artifact 1"))
			Expect(err).NotTo(HaveOccurred())
			Expect(entry.LogIndex).To(BeEquivalentTo(1))
		})

		It("logs concurrent entries through different nodes", func() {
			ctx, cancel := viewContext()
			defer cancel()

			// The specs before logged two entries.
			var wg sync.WaitGroup
			clients := []*client.Client{alice, bob}
			entries := make([]*rekorv1.TransparencyLogEntry, 2)
			errs := make([]error, 2)
			for i, c := range clients {
				wg.Go(func() {
					defer GinkgoRecover()
					entries[i], errs[i] = c.Log(ctx, []byte(fmt.Sprintf("concurrent %d", i)))
				})
			}
			wg.Wait()
			Expect(errs).To(HaveEach(BeNil()))
			Expect([]int64{entries[0].LogIndex, entries[1].LogIndex}).To(ConsistOf(BeEquivalentTo(2), BeEquivalentTo(3)))

			// The size grew by exactly 2: the next entry lands at index 4.
			entry, err := logArtifact(s.II, "org3", []byte("after the concurrent ones"), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(entry.LogIndex).To(BeEquivalentTo(4))
		})

		It("serves the same log through every node", func() {
			ctx, cancel := viewContext()
			defer cancel()

			before, err := checkpointSize(s.II, "org1", verifiers)
			Expect(err).NotTo(HaveOccurred())

			clients := map[string]*client.Client{"org1": alice, "org2": bob}
			for i, node := range []string{"org1", "org2", "org1"} {
				entry, err := clients[node].Log(ctx, fmt.Appendf(nil, "served %d", i))
				Expect(err).NotTo(HaveOccurred())
				Expect(entry.LogIndex).To(BeEquivalentTo(before + uint64(i)))
			}

			// Index 0 was logged in the very first spec; every node still
			// has it and can prove it against its current, larger root.
			for _, c := range []*client.Client{alice, bob, carol} {
				_, err := c.Get(ctx, 0)
				Expect(err).NotTo(HaveOccurred())
			}

			var roots [][]byte
			for _, node := range nodes {
				resp, err := checkpointOf(s.II, node, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Refusals).To(BeEmpty())
				cp, err := translog.VerifyAll(resp.Checkpoint, verifiers)
				Expect(err).NotTo(HaveOccurred())
				Expect(cp.Size).To(Equal(before + 3))
				roots = append(roots, cp.Hash)
			}
			Expect(roots[1]).To(Equal(roots[0]))
			Expect(roots[2]).To(Equal(roots[0]))
		})

		It("rejects an index outside the log", func() {
			ctx, cancel := viewContext()
			defer cancel()

			size, err := checkpointSize(s.II, "org1", verifiers)
			Expect(err).NotTo(HaveOccurred())

			_, err = alice.Get(ctx, size)
			Expect(err).To(HaveOccurred())
		})

		It("audits the log as it grows", func() {
			ctx, cancel := viewContext()
			defer cancel()

			// Each Log ends with an Audit, so alice's first Audit here sees
			// a smaller log than her second: the explicit Audit below
			// exercises the consistency-proof path against that earlier
			// checkpoint.
			_, err := alice.Log(ctx, []byte("growing alice"))
			Expect(err).NotTo(HaveOccurred())
			_, err = bob.Log(ctx, []byte("growing bob"))
			Expect(err).NotTo(HaveOccurred())
			Expect(alice.Audit(ctx)).To(Succeed())

			aliceCp, err := translog.VerifyAll(alice.Latest, verifiers)
			Expect(err).NotTo(HaveOccurred())
			bobCp, err := translog.VerifyAll(bob.Latest, verifiers)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliceCp.Size).To(Equal(bobCp.Size))
		})

		// org3 is the cheating node: its cheat view switches its get and
		// checkpoint views to lie, and carol, who talks to org3, has to
		// catch it.
		It("catches a node that rewrites an entry", func() {
			ctx, cancel := viewContext()
			defer cancel()
			logAtLeast(s.II, 3, verifiers)

			setCheat(s.II, views.CheatRewriteEntry)
			defer setCheat(s.II, views.CheatHonest)
			_, err := carol.Get(ctx, 1)
			Expect(err).To(MatchError(ContainSubstring("failed verifying the inclusion proof")))

			// org1 is honest and still proves the same entry.
			_, err = alice.Get(ctx, 1)
			Expect(err).NotTo(HaveOccurred())
		})

		It("catches a node that forks the log", func() {
			ctx, cancel := viewContext()
			defer cancel()
			logAtLeast(s.II, 3, verifiers)
			Expect(carol.Audit(ctx)).To(Succeed())

			setCheat(s.II, views.CheatFork)
			defer setCheat(s.II, views.CheatHonest)
			Expect(carol.Audit(ctx)).To(MatchError(ContainSubstring("missing signature")))

			// org1 and org2 saw a root that is not theirs at that size.
			resp, err := checkpointOf(s.II, "org3", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Refusals).To(Equal([]string{views.RefusedMismatch, views.RefusedMismatch}))

			// Honest again, org3's checkpoint is consistent with the one
			// carol audited before the fork.
			setCheat(s.II, views.CheatHonest)
			Expect(carol.Audit(ctx)).To(Succeed())
		})
	})
})

// viewContext returns a context bounded by viewTimeout, for a client call.
func viewContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), viewTimeout)
}

// newClient returns a client for node, with a fresh artifact-signing key
// and verifiers for all three orgs' checkpoint keys.
func newClient(ii *integration.Infrastructure, node string, verifiers []note.Verifier) *client.Client {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	return &client.Client{Node: ii.Client(node), Key: key, Verifiers: verifiers}
}

// initNodes hands every node a fresh log key, the three public keys and its
// two peers, and returns the verifiers of the three keys.
func initNodes(ii *integration.Infrastructure) []note.Verifier {
	keys := make([]*ecdsa.PrivateKey, len(nodes))
	pubs := make([][]byte, len(nodes))
	verifiers := make([]note.Verifier, len(nodes))
	for i := range nodes {
		var err error
		keys[i], err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).NotTo(HaveOccurred())
		pubs[i], err = x509.MarshalPKIXPublicKey(&keys[i].PublicKey)
		Expect(err).NotTo(HaveOccurred())
		verifiers[i], err = translog.NewVerifier(&keys[i].PublicKey)
		Expect(err).NotTo(HaveOccurred())
	}
	for i, node := range nodes {
		der, err := x509.MarshalPKCS8PrivateKey(keys[i])
		Expect(err).NotTo(HaveOccurred())
		var peers []view.Identity
		for _, peer := range nodes {
			if peer != node {
				peers = append(peers, ii.Identity(peer))
			}
		}
		_, err = call(ii, node, "init", &views.Setup{LogKey: der, Verifiers: pubs, Peers: peers})
		Expect(err).NotTo(HaveOccurred())
	}
	return verifiers
}

// logArtifact logs artifact through node, signed by a fresh client key. If
// signed is not nil, the signature covers signed instead of artifact. Used
// to craft a request no honest client would.
func logArtifact(ii *integration.Infrastructure, node string, artifact, signed []byte) (*rekorv1.TransparencyLogEntry, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	Expect(err).NotTo(HaveOccurred())
	if signed == nil {
		signed = artifact
	}
	signedDigest := sha256.Sum256(signed)
	sig, err := ecdsa.SignASN1(rand.Reader, key, signedDigest[:])
	Expect(err).NotTo(HaveOccurred())
	digest := sha256.Sum256(artifact)
	req, err := protojson.Marshal(translog.NewRequest(digest[:], sig, pub))
	Expect(err).NotTo(HaveOccurred())

	raw, err := call(ii, node, "log", &views.LogRequest{Request: req})
	if err != nil {
		return nil, err
	}
	entry := &rekorv1.TransparencyLogEntry{}
	Expect(protojson.Unmarshal(raw, entry)).To(Succeed())
	return entry, nil
}

// setCheat sets org3's cheat mode. A spec that cheats defers setting it
// back to "", so no later spec or rerun meets a cheating org3. (A
// DeferCleanup would run after AfterAll tore the network down.)
func setCheat(ii *integration.Infrastructure, mode string) {
	_, err := call(ii, "org3", "cheat", &views.CheatRequest{Mode: mode})
	Expect(err).NotTo(HaveOccurred())
}

// logAtLeast logs entries through org1 until the log holds at least n.
func logAtLeast(ii *integration.Infrastructure, n uint64, vs []note.Verifier) {
	size, err := checkpointSize(ii, "org1", vs)
	Expect(err).NotTo(HaveOccurred())
	for i := size; i < n; i++ {
		_, err := logArtifact(ii, "org1", fmt.Appendf(nil, "filler %d", i), nil)
		Expect(err).NotTo(HaveOccurred())
	}
}

// checkpointOf calls the checkpoint view on node and decodes its result.
func checkpointOf(ii *integration.Infrastructure, node string, oldSize uint64) (*views.CheckpointResponse, error) {
	raw, err := call(ii, node, "checkpoint", &views.CheckpointRequest{OldSize: oldSize})
	if err != nil {
		return nil, err
	}
	resp := &views.CheckpointResponse{}
	Expect(json.Unmarshal(raw, resp)).To(Succeed())
	return resp, nil
}

// checkpointSize returns the current log size as node's checkpoint reports
// it, verified against vs.
func checkpointSize(ii *integration.Infrastructure, node string, vs []note.Verifier) (uint64, error) {
	resp, err := checkpointOf(ii, node, 0)
	if err != nil {
		return 0, err
	}
	cp, err := translog.VerifyAll(resp.Checkpoint, vs)
	if err != nil {
		return 0, err
	}
	return cp.Size, nil
}

// call runs the view registered as fid on node and returns its raw result.
func call(ii *integration.Infrastructure, node, fid string, in any) ([]byte, error) {
	ctx, cancel := viewContext()
	defer cancel()
	res, err := ii.Client(node).CallViewWithContext(ctx, fid, common.JSONMarshall(in))
	if err != nil {
		return nil, err
	}
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue(), "unexpected view result type %T", res)
	return raw, nil
}
