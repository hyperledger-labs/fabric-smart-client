/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/fake"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	libp2psupport "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/support/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	client3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	client2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
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

		It("successful pingpong based on REST API", func() {
			// Init and Start fsc nodes
			initiator = newNode("./testdata/fsc/nodes/initiator.0")
			responder = newNode("./testdata/fsc/nodes/responder.0")

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = view2.GetRegistry(initiator).RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			Expect(view2.GetRegistry(responder).RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)

			webClientConfig, err := client.NewWebClientConfigFromFSC("./testdata/fsc/nodes/initiator.0")
			Expect(err).NotTo(HaveOccurred())
			initiatorWebClient, err := client2.NewClient(webClientConfig)
			Expect(err).NotTo(HaveOccurred())
			res, err := initiatorWebClient.CallView("init", common.JSONMarshall(&pingpong.Params{Rounds: 5}))
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

			webClientConfig.TLSCertPath = ""
			initiatorWebClient, err = client2.NewClient(webClientConfig)
			Expect(err).NotTo(HaveOccurred())
			_, err = initiatorWebClient.CallView("init", common.JSONMarshall(&pingpong.Params{Rounds: 5}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status code [401], status [401 Unauthorized]"))
		})

		It("successful pingpong based on WebSocket", func() {
			// Init and Start fsc nodes
			initiator = newNode("./testdata/fsc/nodes/initiator.0")
			Expect(initiator).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = view2.GetRegistry(initiator).RegisterFactory("stream", &pingpong.StreamerViewFactory{})
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

		It("successful pingpong", func() {
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
			err = view2.GetRegistry(initiator).RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			Expect(view2.GetRegistry(responder).RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)
			// Initiate a view and check the output
			res, err := client3.NewLocalClient(initiator).CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})
	})

	// Loading runs libp2p whatever transport it is handed -- the committed
	// ./testdata fixture is type: libp2p and fsc.Platform.Load never rewrites it.
	// Hence one loading group, labelled for what it runs; see
	// docs/agents/integration-tests.md#choosing-a-p2p-comm-type.
	Describe("Network-based Ping pong, loaded artifacts", Label(fsc.LibP2P), Ordered, func() {
		s := NewTestSuite(fsc.LibP2P, doLoad, integration.NoReplication)
		BeforeAll(s.Setup)
		AfterAll(s.TearDown)
		It("successful pingpong", func() { s.TestPingPong("initiator") })
		It("successful pingpong with stream", func() { s.TestPingPongStream("initiator") })
		It("successful stream", func() { s.TestStream("initiator") })
		It("successful stream with websocket", func() { s.TestStreamWebsocket("initiator") })
		It("init clients & successful pingpong", s.TestLoadInitPingPong)
	})

	// Generating honours the transport, so both get a group. What it adds over the
	// loading group is config writing and node bootstrap, not client API surface.
	for _, c := range []fsc.P2PCommunicationType{fsc.LibP2P, fsc.WebSocket} {
		Describe("Network-based Ping pong, generated artifacts", Label(c), Ordered, func() {
			s := NewTestSuite(c, doGenerate, integration.NoReplication)
			BeforeAll(s.Setup)
			AfterAll(s.TearDown)
			It("successful pingpong", func() { s.TestPingPong("initiator") })
			It("successful mock pingpong", s.TestMockPingPong)
		})
	}

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
		It("successful pingpong", func() { s.TestPingPong(initiatorReplicas...) })
		It("successful pingpong with stream", func() { s.TestPingPongStream(initiatorReplicas...) })
		It("successful stream", func() { s.TestStream(initiatorReplicas...) })
		It("successful stream with websocket", func() { s.TestStreamWebsocket(initiatorReplicas...) })
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
		var channel *client2.WSStream
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

func newWebClient(confDir string) *client2.Client {
	c, err := client.NewWebClientConfigFromFSC(confDir)
	Expect(err).NotTo(HaveOccurred())
	initiator, err := client2.NewClient(c)
	Expect(err).NotTo(HaveOccurred())
	return initiator
}

func GetFSCReplicaNames(nodeName string, replicationFactor int) []string {
	result := make([]string, replicationFactor)
	for i := range replicationFactor {
		result[i] = fmt.Sprintf("fsc.%s.%d", nodeName, i)
	}
	return result
}
