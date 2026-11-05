/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/fake"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	libp2psupport "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/support/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	viewclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var _ = Describe("EndToEnd", func() {
	// libp2p, not websocket: these specs start in-process nodes straight from the
	// committed testdata under ./testdata, whose config is libp2p. It cannot be
	// switched to websocket by editing the config alone -- the websocket host
	// derives its P2P TLS material from fsc.identity.cert.file, and this fixture's
	// signing cert carries no IP SANs, so a handshake to 127.0.0.1 cannot verify.
	// Making it websocket means regenerating the fixture's crypto.
	Describe("Node-based Ping pong", Label(fsc.LibP2P), func() {
		var (
			initiator FSCNode
			responder FSCNode
		)

		AfterEach(func() {
			// Stop the ii
			initiator.Stop()
			if responder != nil {
				responder.Stop()
			}
			time.Sleep(5 * time.Second)
		})

		It("pings over the REST API and rejects an unauthorized client", func() {
			// Init and Start fsc nodes
			initiator = newNode("./testdata/fsc/nodes/initiator.0")
			responder = newNode("./testdata/fsc/nodes/responder.0")

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = view.GetRegistry(initiator).RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			Expect(view.GetRegistry(responder).RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)

			initiatorWebClient := newWebClient("./testdata/fsc/nodes/initiator.0")
			res, err := initiatorWebClient.CallView("init", common.JSONMarshall(&pingpong.Params{Rounds: 5}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

			// A command signed by an identity the initiator does not authorize is rejected:
			// the responder's identity is a valid certificate of the same organization, but
			// neither the initiator's own nor on its fsc.view.remote.clients.
			crypto := "./testdata/fsc/crypto/peerOrganizations/fsc.example.com/peers/responder.fsc.example.com/msp/"
			stranger, err := viewclient.NewX509SigningIdentity(crypto+"signcerts/responder.fsc.example.com-cert.pem", crypto+"keystore/priv_sk")
			Expect(err).NotTo(HaveOccurred())
			initiatorWebClient.SigningIdentity = stranger
			_, err = initiatorWebClient.CallView("init", common.JSONMarshall(&pingpong.Params{Rounds: 5}))
			Expect(err).To(MatchError(ContainSubstring("not recognized")))
		})

		It("enforces a per-view ACL from a policy checker", func() {
			// The initiator's ACL lets its own identity call every view, the Admin client (on
			// fsc.view.remote.clients) only "init", and the responder's identity, which is not
			// a client, no view.
			initiator = newNodeWithACL("./testdata/fsc/nodes/initiator.0", viewACL{
				readCert("users/Admin@fsc.example.com", "Admin@fsc.example.com"):         {"init"},
				readCert("peers/responder.fsc.example.com", "responder.fsc.example.com"): {},
			})
			responder = newNode("./testdata/fsc/nodes/responder.0")
			Expect(initiator.Start()).To(Succeed())
			Expect(responder.Start()).To(Succeed())
			Expect(view.GetRegistry(initiator).RegisterFactory("init", &pingpong.InitiatorViewFactory{})).To(Succeed())
			Expect(view.GetRegistry(initiator).RegisterFactory("stream", &pingpong.StreamerViewFactory{})).To(Succeed())
			Expect(view.GetRegistry(responder).RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})).To(Succeed())
			time.Sleep(3 * time.Second)

			client := newWebClient("./testdata/fsc/nodes/initiator.0")
			stream, err := client.StreamCallView("stream", nil)
			Expect(err).NotTo(HaveOccurred())
			var s string
			Expect(stream.Recv(&s)).To(Succeed())
			Expect(stream.Send("ciao")).To(Succeed())
			_, err = stream.Result()
			Expect(err).NotTo(HaveOccurred(), "a stream from the node identity passes the ACL")

			client.SigningIdentity = signingIdentity("users/Admin@fsc.example.com", "Admin@fsc.example.com")
			res, err := client.CallView("init", common.JSONMarshall(&pingpong.Params{Rounds: 1}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
			_, err = client.CallView("stream", nil)
			Expect(err).To(MatchError(And(ContainSubstring("access denied"), ContainSubstring("view [stream]"))))

			// The access control check refuses a signer that is not a client before the ACL
			// runs; the ACL would answer "access denied".
			client.SigningIdentity = signingIdentity("peers/responder.fsc.example.com", "responder.fsc.example.com")
			_, err = client.CallView("init", nil)
			Expect(err).To(MatchError(ContainSubstring("not recognized")))
		})

		It("refuses a responder to a node its authorizer denies", func() {
			initiator = newNode("./testdata/fsc/nodes/initiator.0")
			responder = newNode("./testdata/fsc/nodes/responder.0")
			Expect(initiator.Start()).To(Succeed())
			Expect(responder.Start()).To(Succeed())
			Expect(view.GetRegistry(initiator).RegisterFactory("init", &pingpong.InitiatorViewFactory{})).To(Succeed())

			// The authorizer gets the initiator's node identity, as the transport authenticated it.
			ip, err := id.GetProvider(initiator)
			Expect(err).NotTo(HaveOccurred())
			denied := id.CanonicalIdentity(ip.DefaultIdentity())
			refuse := view.AuthorizerFunc(func(_ context.Context, caller viewapi.Identity) error {
				if bytes.Equal(id.CanonicalIdentity(caller), denied) {
					return errors.Wrapf(view.ErrDenied, "node [%s]", caller)
				}
				return nil
			})
			Expect(view.GetRegistry(responder).RegisterResponderWithOptions(&pingpong.Responder{}, &pingpong.Initiator{}, view.WithAuthorizer(refuse))).To(Succeed())
			time.Sleep(3 * time.Second)

			start := time.Now()
			_, err = viewclient.NewLocalClient(initiator).CallView("init", nil)
			// The initiator's view fails with the responder's rejection; typed errors do not
			// cross the view API, so match the text.
			Expect(err).To(MatchError(ContainSubstring("unauthorized: caller may not start this view")))
			Expect(time.Since(start)).To(BeNumerically("<", 10*time.Second))
		})

		It("streams a view call over the web client", func() {
			// Init and Start fsc nodes
			initiator = newNode("./testdata/fsc/nodes/initiator.0")
			Expect(initiator).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = view.GetRegistry(initiator).RegisterFactory("stream", &pingpong.StreamerViewFactory{})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)

			initiatorWebClient := newWebClient("./testdata/fsc/nodes/initiator.0")
			stream, err := initiatorWebClient.StreamCallView("stream", nil)
			Expect(err).NotTo(HaveOccurred())
			var s string
			Expect(stream.Recv(&s)).NotTo(HaveOccurred())
			Expect(s).To(BeEquivalentTo("hello"))
			Expect(stream.Send("ciao")).NotTo(HaveOccurred())

			res, err := stream.Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("pings through a local client", func() {
			// Init and Start fsc nodes
			initiator = newNode("./testdata/fsc/nodes/initiator.0")
			Expect(initiator).NotTo(BeNil())

			responder = newNode("./testdata/fsc/nodes/responder.0")
			Expect(responder).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = view.GetRegistry(initiator).RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			Expect(view.GetRegistry(responder).RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)
			// Initiate a view and check the output
			res, err := viewclient.NewLocalClient(initiator).CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})
	})

	// Loading runs libp2p whatever transport it is handed -- the committed
	// ./testdata fixture is type: libp2p and fsc.Platform.Load never rewrites it.
	// Hence one loading group, labelled for what it runs; see
	// docs/dev/integration-tests/authoring.md#choosing-a-p2p-comm-type.
	Describe("Network-based Ping pong, loaded artifacts", Label(fsc.LibP2P), Ordered, func() {
		s := NewTestSuite(fsc.LibP2P, doLoad, integration.NoReplication)
		BeforeAll(s.Setup)
		AfterAll(s.TearDown)
		It("pings the responder", func() { s.TestPingPong("initiator") })
		It("pings the responder through a streamed view call", func() { s.TestPingPongStream("initiator") })
		It("exchanges messages on a view stream", func() { s.TestStream("initiator") })
		It("exchanges messages on a view stream over the web client", func() { s.TestStreamWebsocket("initiator") })
		It("pings through clients built from the loaded artifacts", s.TestLoadInitPingPong)
	})

	// Generating honours the transport, so both get a group. What it adds over the
	// loading group is config writing and node bootstrap, not client API surface.
	for _, c := range []fsc.P2PCommunicationType{fsc.LibP2P, fsc.WebSocket} {
		Describe("Network-based Ping pong, generated artifacts", Label(c), Ordered, func() {
			s := NewTestSuite(c, doGenerate, integration.NoReplication)
			BeforeAll(s.Setup)
			AfterAll(s.TearDown)
			It("pings the responder", func() { s.TestPingPong("initiator") })
			It("gets a mock pong only from a mocked responder", s.TestMockPingPong)
			It("fails at once on a view the responder does not host", s.TestUnhostedView)
			if c == fsc.WebSocket {
				It("checks its peers with node peers --check", s.TestPeersCheck)
			}
			It("negotiates the highest version both nodes support", s.TestVersion)
		})
	}

	// The initiator re-reads its TLS certificate and key every second. The group has a network
	// of its own, torn down after its spec.
	Describe("Ping pong with TLS certificate reload", Label(fsc.WebSocket), Ordered, func() {
		s := &TestSuite{TestSuite: integration.NewTestSuite(func() (*integration.Infrastructure, error) {
			ii, err := integration.Generate(StartPortWithGeneration(), integration.WithRaceDetection, pingpong.TLSReloadTopology(fsc.WebSocket)...)
			if err != nil {
				return nil, err
			}
			ii.DeleteOnStop = false
			return ii, nil
		})}
		BeforeAll(s.Setup)
		AfterAll(s.TearDown)
		It("serves a renewed TLS certificate without a restart", s.TestTLSCertReload)
	})

	// Replication generates, so this group is genuinely websocket.
	Describe("Network-based Ping pong With Websockets and replication", Label(fsc.WebSocket), Ordered, func() {
		s := NewTestSuite(fsc.WebSocket, doGenerate, &integration.ReplicationOptions{
			ReplicationFactors: map[string]int{
				"initiator": 3,
			},
		})
		initiatorReplicas := GetFSCReplicaNames("initiator", 3)
		BeforeAll(s.Setup)
		AfterAll(s.TearDown)
		It("pings the responder", func() { s.TestPingPong(initiatorReplicas...) })
		It("pings the responder through a streamed view call", func() { s.TestPingPongStream(initiatorReplicas...) })
		It("exchanges messages on a view stream", func() { s.TestStream(initiatorReplicas...) })
		It("exchanges messages on a view stream over the web client", func() { s.TestStreamWebsocket(initiatorReplicas...) })
	})
})

type FSCNode interface {
	Stop()
	Start() error
	GetService(v any) (any, error)
}

func newNode(conf string) FSCNode {
	n := node.NewFromConfPath(conf)
	Expect(n).NotTo(BeNil())
	n.AddSDK(libp2psupport.NewFrom(viewsdk.NewSDK(n)))
	return n
}

const cryptoDir = "./testdata/fsc/crypto/peerOrganizations/fsc.example.com/"

// readCert returns the PEM certificate of the MSP under cryptoDir+dir, which is the creator of
// the commands its signing identity signs.
func readCert(dir, name string) string {
	raw, err := os.ReadFile(cryptoDir + dir + "/msp/signcerts/" + name + "-cert.pem")
	Expect(err).NotTo(HaveOccurred())
	return string(raw)
}

// signingIdentity loads the signing identity of the MSP under cryptoDir+dir.
func signingIdentity(dir, name string) viewclient.SigningIdentity {
	msp := cryptoDir + dir + "/msp/"
	si, err := viewclient.NewX509SigningIdentity(msp+"signcerts/"+name+"-cert.pem", msp+"keystore/priv_sk")
	Expect(err).NotTo(HaveOccurred())
	return si
}

// newNodeWithACL returns a node whose view API lets each creator in acl call only the views
// listed for it, and any other creator call every view.
func newNodeWithACL(conf string, acl viewACL) FSCNode {
	n := node.NewFromConfPath(conf)
	Expect(n).NotTo(BeNil())
	n.AddSDK(&aclSDK{SDK: libp2psupport.NewFrom(viewsdk.NewSDK(n)), acl: acl})
	return n
}

// aclSDK stands for an application SDK that adds a policy checker to the view API.
type aclSDK struct {
	commondig.SDK
	acl viewACL
}

func (s *aclSDK) Install() error {
	if err := s.Container().Provide(func() viewsdk.PolicyCheckerEntry {
		return viewsdk.PolicyCheckerEntry{Checker: s.acl}
	}); err != nil {
		return err
	}
	return s.SDK.Install()
}

// viewACL maps a creator to the views it may call. It governs view calls only.
type viewACL map[string][]string

func (a viewACL) Check(_ *protos.SignedCommand, c *protos.Command) error {
	call := c.GetCallView()
	if call == nil {
		return nil
	}
	allowed, limited := a[string(c.GetHeader().GetCreator())]
	if !limited || slices.Contains(allowed, call.GetFid()) {
		return nil
	}
	return errors.Wrapf(view.ErrDenied, "view [%s] not allowed", call.GetFid())
}

const testdataDir = "./testdata"

// Named because `NewTestSuite(commType, true, opts)` reads as neither.
const (
	doGenerate = true
	doLoad     = false
)

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, generate bool, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{
		TestSuite: integration.NewTestSuite(func() (ii *integration.Infrastructure, err error) {
			topologies := pingpong.Topology(commType, nodeOpts)
			// Independent paths: Generate writes a fresh config into a temp dir,
			// Load reads ./testdata and leaves it alone.
			if generate {
				ii, err = integration.Generate(StartPortWithGeneration(), integration.WithRaceDetection, topologies...)
			} else {
				ii, err = integration.Load(0, testdataDir, integration.WithRaceDetection, topologies...)
			}
			if err != nil {
				// Both constructors return a nil Infrastructure on failure.
				return nil, err
			}
			ii.DeleteOnStop = false
			return ii, nil
		}),
	}
}

func (s *TestSuite) TestPingPong(clients ...string) {
	// Initiate a view and check the output
	for _, clientName := range clients {
		res, err := s.II.Client(clientName).CallView("init", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestPingPongStream(clients ...string) {
	// Initiate a view and check the output
	for _, clientName := range clients {
		channel, err := s.II.Client(clientName).StreamCallView("init", nil)
		Expect(err).NotTo(HaveOccurred())

		res, err := channel.Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestStream(clients ...string) {
	for _, clientName := range clients {
		channel, err := s.II.Client(clientName).StreamCallView("stream", nil)
		Expect(err).NotTo(HaveOccurred())
		var str string
		Expect(channel.Recv(&str)).NotTo(HaveOccurred())
		Expect(str).To(BeEquivalentTo("hello"))
		Expect(channel.Send("ciao")).NotTo(HaveOccurred())

		res, err := channel.Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestStreamWebsocket(clients ...string) {
	for _, clientName := range clients {
		// Get a client for the fsc node labelled initiator
		initiator := s.II.WebClient(clientName)
		// The node's web server may still be coming up. Poll the stream call
		// itself rather than a separate endpoint: this suite enables only the web
		// server (topology.WebEnabled), not Prometheus metrics, so there is no
		// /metrics route to probe. An attempt that errors yields no usable
		// stream, so retrying is safe.
		var channel *viewclient.Stream
		Eventually(func() error {
			var err error
			channel, err = initiator.StreamCallView("stream", nil)
			return err
		}, 30*time.Second, 250*time.Millisecond).Should(Succeed())
		var str string
		Expect(channel.Recv(&str)).NotTo(HaveOccurred())
		Expect(str).To(BeEquivalentTo("hello"))
		Expect(channel.Send("ciao")).NotTo(HaveOccurred())

		res, err := channel.Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestLoadInitPingPong() {
	// Use another ii to create clients
	iiClients, err := integration.Clients(testdataDir, s.II.Topologies...)
	Expect(err).NotTo(HaveOccurred())

	// Get a client for the fsc node labelled initiator
	initiator := iiClients.Client("initiator")
	// Initiate a view and check the output
	res, err := initiator.CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestMockPingPong() {
	// Init with mock=false, a failure must happen
	_, err := s.II.Client("initiator").CallView("mockInit", common.JSONMarshall(&fake.Params{Mock: false}))
	Expect(err).To(HaveOccurred())
	Expect(strings.Contains(err.Error(), "expected mock pong, got pong")).To(BeTrue())

	// Init with mock=true, a success must happen
	res, err := s.II.Client("initiator").CallView("mockInit", common.JSONMarshall(&fake.Params{Mock: true}))
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

// TestPeersCheck runs `node peers --check` from the initiator's binary and configuration
// against the running responder: their ranges overlap, so it exits 0.
func (s *TestSuite) TestPeersCheck() {
	session := s.II.RunFSCNodeCommand("initiator", "node", "peers", "--check")
	Expect(session.ExitCode()).To(BeZero(), string(session.Err.Contents()))
	Expect(string(session.Out.Contents())).To(ContainSubstring("peer [responder]"))
	// The command starts no node, yet sees the component the binary registers in main.
	Expect(string(session.Out.Contents())).To(ContainSubstring("pingtest: local [2,3] peer [1,2]: uses 2"))
}

// TestUnhostedView checks that a view the responder hosts no responder for fails at once with
// the responder's unknown_view rejection, instead of waiting for its session to time out.
func (s *TestSuite) TestUnhostedView() {
	start := time.Now()
	_, err := s.II.Client("initiator").CallView("unhosted", nil)
	Expect(err).To(MatchError(ContainSubstring("unknown_view: github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/Unhosted")))
	// Well under roundTimeout (one minute), which the call takes without the rejection.
	Expect(time.Since(start)).To(BeNumerically("<", 10*time.Second))
}

// TestVersion checks that an initiator supporting pingtest {2, 3} and a responder supporting
// {1, 2} both observe version 2, the highest both support, on their session. The Initiator's
// ping-pong in the other specs reaches its responder by the protocol name pingpong/ping.
func (s *TestSuite) TestVersion() {
	res, err := s.II.Client("initiator").CallView("version", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(2))
}

// newWebClient returns a view client that reaches the node in confDir over HTTP, signed as
// the node itself.
func newWebClient(confDir string) *viewclient.Client {
	c, err := client.NewHTTPViewClient(confDir, noop.NewTracerProvider())
	Expect(err).NotTo(HaveOccurred())
	return c
}

func GetFSCReplicaNames(nodeName string, replicationFactor int) []string {
	result := make([]string, replicationFactor)
	for i := range replicationFactor {
		result[i] = fmt.Sprintf("fsc.%s.%d", nodeName, i)
	}
	return result
}

// TestTLSCertReload renews the initiator's TLS certificate on disk and checks that its gRPC and
// web listeners serve the renewed one within a few reload intervals, without a restart: the
// expiry gauge of both moves, a new web client calls a view, and the initiator still reaches
// the responder over P2P. The P2P transport does not reload, so that ping is a liveness check
// and does not use the renewed certificate. A mismatched pair written next is refused, so the
// gauge keeps the renewed expiry and a new client still connects. Writing the original pair
// back last checks that a reload also returns to an earlier certificate.
func (s *TestSuite) TestTLSCertReload() {
	p := s.II.FscPlatform
	initiator := p.Peer("fsc", "initiator")
	Expect(initiator).NotTo(BeNil())
	n := tlsNode{
		dir:      p.NodeDir(nwonode.NewReplica(initiator, nwonode.ReplicaUniqueName("initiator", 0))),
		identity: func() (viewclient.SigningIdentity, error) { return p.GetSigningIdentity(initiator) },
	}
	listeners := []string{"grpc", "web"}
	// Each reload re-reads the files, so a few intervals cover a reload that just missed a write.
	reloaded := 5 * pingpong.TLSReloadInterval

	original, err := p.NodeTLSPair(initiator)
	Expect(err).NotTo(HaveOccurred())
	before := n.expectCertNotAfter(original, listeners, notAfterOf(original), reloaded)

	By("renewing the initiator's certificate")
	renewed, err := p.RenewNodeTLSPair(initiator, before.Add(24*time.Hour).Truncate(time.Second))
	Expect(err).NotTo(HaveOccurred())
	Expect(p.WriteNodeTLSPair(initiator, renewed)).To(Succeed())
	n.expectCertNotAfter(renewed, listeners, notAfterOf(renewed), reloaded)
	n.expectNewWebClientCalls(renewed)
	s.TestPingPong("initiator")

	By("writing a mismatched pair, which the initiator refuses")
	// The renewed certificate stays on disk and the original key replaces its key, so the
	// node's files no longer match. The clients present the renewed pair, which the node keeps
	// serving, since they cannot load the node's files either.
	Expect(p.WriteNodeTLSPair(initiator, fsc.TLSPair{Cert: renewed.Cert, Key: original.Key})).To(Succeed())
	Consistently(func(g Gomega) {
		got := n.certNotAfter(g, renewed)
		for _, l := range listeners {
			g.Expect(got).To(HaveKeyWithValue(l, notAfterOf(renewed)), "listener [%s]", l)
		}
	}, reloaded, pingpong.TLSReloadInterval/2).Should(Succeed())
	n.expectNewWebClientCalls(renewed)

	By("writing the original pair back, which the initiator serves again")
	Expect(p.WriteNodeTLSPair(initiator, original)).To(Succeed())
	n.expectCertNotAfter(original, listeners, notAfterOf(original), reloaded)
	n.expectNewWebClientCalls(original)
}

// tlsNode is a node whose TLS certificate a spec rewrites. Its clients are built for each check
// from the node's configuration, with the pair the spec expects the node to serve in place of the
// one its files name: a client built earlier trusts the certificate the node served then, and a
// mismatched pair on disk cannot be loaded at all.
type tlsNode struct {
	dir      string
	identity func() (viewclient.SigningIdentity, error)
}

// withPair makes cc trust pair's certificate and present pair, as nwo's clients do with the
// node's own pair.
func withPair(cc *commongrpc.ConnectionConfig, pair fsc.TLSPair) *commongrpc.ConnectionConfig {
	cc.TLS.ServerRootCAs = [][]byte{pair.Cert}
	cc.TLS.Certificate, cc.TLS.Key = pair.Cert, pair.Key
	return cc
}

// certNotAfter returns the node's fsc_tls_cert_not_after by listener, read over a new
// connection that trusts pair.
func (n tlsNode) certNotAfter(g Gomega, pair fsc.TLSPair) map[string]time.Time {
	cc, err := client.NewMetricsClientConfigFromFSC(n.dir)
	g.Expect(err).NotTo(HaveOccurred())
	mc, err := client.NewMetricsClient(withPair(cc, pair))
	g.Expect(err).NotTo(HaveOccurred())
	families, err := mc.Metrics()
	g.Expect(err).NotTo(HaveOccurred())
	return fsc.ParseTLSCertNotAfter(families)
}

// expectCertNotAfter waits until every listener in listeners reports want as the expiry of the
// certificate it serves, read over connections that trust pair, and returns want.
func (n tlsNode) expectCertNotAfter(pair fsc.TLSPair, listeners []string, want time.Time, timeout time.Duration) time.Time {
	EventuallyWithOffset(1, func(g Gomega) {
		got := n.certNotAfter(g, pair)
		for _, l := range listeners {
			g.Expect(got).To(HaveKeyWithValue(l, want), "listener [%s]", l)
		}
	}, timeout, 200*time.Millisecond).Should(Succeed())
	return want
}

// expectNewWebClientCalls calls a view over a new web connection that trusts pair, so the
// handshake verifies the certificate the node serves now.
func (n tlsNode) expectNewWebClientCalls(pair fsc.TLSPair) {
	cc, err := client.NewWebConnectionConfigFromFSC(n.dir)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	signer, err := n.identity()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	c, err := viewclient.NewClient(&viewclient.Config{ConnectionConfig: withPair(cc, pair), Transport: viewclient.TransportHTTP}, signer, noop.NewTracerProvider())
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	res, err := c.CallView("init", nil)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

// notAfterOf returns the expiry of the certificate in pair.
func notAfterOf(pair fsc.TLSPair) time.Time {
	block, _ := pem.Decode(pair.Cert)
	ExpectWithOffset(1, block).NotTo(BeNil())
	cert, err := x509.ParseCertificate(block.Bytes)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return cert.NotAfter.UTC()
}
