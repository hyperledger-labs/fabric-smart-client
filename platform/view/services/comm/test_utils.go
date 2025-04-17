/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	endpoint2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

type HostNode struct {
	*P2PNode
	ID      host.PeerID
	Address host.PeerIPAddress
}

type Node struct {
	commService *Service
	address     string
	pkID        view2.Identity
}

func P2PLayerTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		messages := bootstrapNode.incomingMessages

		info := host.StreamInfo{
			RemotePeerID:      node.ID,
			RemotePeerAddress: node.Address,
			ContextID:         "context",
			SessionID:         "session",
		}
		err := bootstrapNode.sendTo(context.Background(), info, &ViewPacket{Payload: []byte("msg1")})
		assert.NoError(t, err)

		err = bootstrapNode.sendTo(context.Background(), info, &ViewPacket{Payload: []byte("msg2")})
		assert.NoError(t, err)

		msg := <-messages
		assert.NotNil(t, msg)
		assert.Equal(t, []byte("msg3"), msg.message.Payload)
	}()

	messages := node.incomingMessages
	msg := <-messages
	assert.NotNil(t, msg)
	assert.Equal(t, []byte("msg1"), msg.message.Payload)

	msg = <-messages
	assert.NotNil(t, msg)
	assert.Equal(t, []byte("msg2"), msg.message.Payload)

	info := host.StreamInfo{
		RemotePeerID:      bootstrapNode.ID,
		RemotePeerAddress: bootstrapNode.Address,
		ContextID:         "context",
		SessionID:         "session",
	}
	err := node.sendTo(context.Background(), info, &ViewPacket{Payload: []byte("msg3")})
	assert.NoError(t, err)

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func SessionsTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		session, err := bootstrapNode.NewSession("", "", node.Address, []byte(node.ID))
		assert.NoError(t, err)
		assert.NotNil(t, session)

		err = session.Send([]byte("ciao"))
		assert.NoError(t, err)

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		err = session.Send([]byte("ciao on session"))
		assert.NoError(t, err)

		session.Close()
	}()

	masterSession, err := node.MasterSession()
	assert.NoError(t, err)
	assert.NotNil(t, masterSession)

	masterSessionMsgs := masterSession.Receive()
	msg := <-masterSessionMsgs
	assert.Equal(t, []byte("ciao"), msg.Payload)

	session, err := node.NewSessionWithID(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, session)

	assert.NoError(t, session.Send([]byte("ciaoback")))

	sessionMsgs := session.Receive()
	msg = <-sessionMsgs
	assert.Equal(t, []byte("ciao on session"), msg.Payload)

	session.Close()

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func SessionsForMPCTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		session, err := bootstrapNode.NewSessionWithID("myawesomempcid", "", bootstrapNode.Address, []byte(node.ID), nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, session)

		err = session.Send([]byte("ciao"))
		assert.NoError(t, err)

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		session.Close()
	}()

	session, err := node.NewSessionWithID("myawesomempcid", "", bootstrapNode.Address, []byte(bootstrapNode.ID), nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, session)

	sessionMsgs := session.Receive()
	msg := <-sessionMsgs
	assert.Equal(t, []byte("ciao"), msg.Payload)

	assert.NoError(t, session.Send([]byte("ciaoback")))

	session.Close()

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func freeAddress() string {
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", utils.MustGet(freeport.Take(1))[0])
}

func GetConfig(name string) *Config {
	rootPath, ok := os.LookupEnv("GOPATH")
	Expect(ok).To(BeTrue(), "GOPATH is not set")
	projectPath := path.Join(rootPath, "src", "github.com", "hyperledger-labs", "fabric-smart-client")
	mspDir := path.Join(projectPath, "integration", "fsc", "pingpong", "testdata", "fsc", "crypto", "peerOrganizations", "fsc.example.com", "peers", fmt.Sprintf("%s.fsc.example.com", name), "msp")
	return &Config{
		ListenAddress: freeAddress(),
		KeyFile:       path.Join(mspDir, "keystore", "priv_sk"),
		CertFile:      path.Join(mspDir, "signcerts", fmt.Sprintf("%s.fsc.example.com-cert.pem", name)),
	}
}

type BootstrapNodeResolver struct {
	nodeAddress string
	nodeID      []byte
}

func (r *BootstrapNodeResolver) Resolve(view2.Identity) (view2.Identity, map[view3.PortName]string, []byte, error) {
	return nil, map[view3.PortName]string{view3.P2PPort: rest.ConvertAddress(r.nodeAddress)}, r.nodeID, nil
}

func (r *BootstrapNodeResolver) GetIdentity(string, []byte) (view2.Identity, error) {
	return view2.Identity("bstpnd"), nil
}

func NewWebsocketCommService(addresses *routing.StaticIDRouter, config *Config) *Service {
	discovery := routing.NewServiceDiscovery(addresses, routing.Random[host.PeerIPAddress]())

	pkiExtractor := endpoint.NewPKIExtractor()
	utils.Must(pkiExtractor.AddPublicKeyExtractor(&PKExtractor{}))
	pkiExtractor.SetPublicKeyIDSynthesizer(&rest.PKIDSynthesizer{})

	hostProvider := rest.NewEndpointBasedProvider(pkiExtractor, discovery, noop.NewTracerProvider(), websocket.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}))

	return newService(hostProvider, nil, config, noop.NewTracerProvider(), &disabled.Provider{})
}

func NewLibP2PCommService(config *Config, resolver EndpointService) (*Service, view2.Identity) {
	pkiExtractor := endpoint.NewPKIExtractor()
	utils.Must(pkiExtractor.AddPublicKeyExtractor(&PKExtractor{}))
	utils.Must(pkiExtractor.AddPublicKeyExtractor(endpoint2.PublicKeyExtractor{}))
	pkiExtractor.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})

	hostProvider := libp2p.NewHostGeneratorProvider(&disabled.Provider{})

	pkID := pkiExtractor.ExtractPKI(utils.MustGet(x509.Serialize("", config.CertFile)))

	return newService(hostProvider, resolver, config, noop.NewTracerProvider(), &disabled.Provider{}), pkID
}
