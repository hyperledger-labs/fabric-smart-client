# Ping Pong

The goal of this section is to familiarise ourselves with the Fabric Smart Client basic concepts.
At the end of this document, we will gain the following knowledge:
- What it means for party to establish a communication channel
- What it means to deploy FSC nodes
- Setup your first FSC environment

But, let us start with the protagonists of our story: Alice and Bob.

## Alice and Bob

Alice and Bob are two parties who know each other and who wants to interact to accomplish a given business task.
Alice is the initiator and starts the interactive protocol by executing a view representing her in the business process.
At some point, Alice needs to send a message to Bob.
To do that, Alice opens a `communication session` to Bob. Alice just needs to know Bob's identity in order to establish
this connection.
When Alice's message reaches Bob, Bob responds by executing a view representing him in the business process.
Bob gets Alice's message, executes his business logic, and  can use the very same session to respond to Alice and
the ping-pong can continue until Alice and Bob reach their goal.

Let us then give a very concrete example of such a ping-pong.
The initiator, Alice, sends a ping to the responder, Bob, and waits for a reply.
Bob, the responder, upon receiving the ping, responds with a pong.

This is view describing Alice's behaviour:

```go
package pingpong

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Initiator struct{}

func (p *Initiator) Call(context view.Context) (interface{}, error) {
	// Retrieve responder identity
	responder := view2.GetIdentityProvider(context).Identity("responder")

	// Open a session to the responder
	session, err := context.GetSession(context.Initiator(), responder)
	assert.NoError(err) 
	// Send a ping
	err = session.Send([]byte("ping"))
	assert.NoError(err) 
	// Wait for the pong
	ch := session.Receive()
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		m := string(msg.Payload)
		if m != "pong" {
			return nil, fmt.Errorf("expected pong, got %s", m)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Return
	return "OK", nil
}
```

Let us go through the main steps:
- **Retrieve responder identity**: The initiator is supposed to send a ping to the responder.
  The first step is therefore to retrieve the responder's identity.
  This can be done by using the identity service.
- **Open a session to the responder**: With the responder's identity, the initiator
  can open a session.
  The context allows the initiator to do that.
- **Send a ping**: Using the established session, the sender sends her ping.
- **Wait for the pong**: At this point, the responder waits for the reply.
  The initiator timeouts if no message comes in a reasonable amount of time.
  If a reply comes, the initiator checks that it contains a pong.
  If this is not the case, the view returns an error.

Let us now look at the view describing the view of the responder:

```go
package pingpong

import (
	"errors"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Responder struct{}

func (p *Responder) Call(context view.Context) (interface{}, error) {
	// Retrieve the session opened by the initiator
	session := context.Session()

	// Read the message from the initiator
	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		payload = msg.Payload
	case <-time.After(5 * time.Second):
		return nil, errors.New("time out reached")
	}

	// Respond with a pong if a ping is received, an error otherwise
	m := string(payload)
	switch {
	case m != "ping":
		// reply with an error
		err := session.SendError([]byte(fmt.Sprintf("expected ping, got %s", m)))
		assert.NoError(err)
		return nil, fmt.Errorf("expected ping, got %s", m)
	default:
		// reply with pong
		err := session.Send([]byte("pong"))
		assert.NoError(err)
	}

	// Return
	return "OK", nil
}
```

These are the  main steps carried on by the responder:
- **Retrieve the session opened by the initiator**: The responder expects to
  be invoked upon the reception of a message transmitted using a session.
  The responder can access his endpoint of this session via the context.
- **Read the message from the initiator**: The responder reads the message, the initiator sent, from
  the session.
- **Respond with a pong if a ping is received, an error otherwise**:
  If the received message is a ping, then the responder replies with a pong.
  Otherwise, the responder replies with an error.

## View Management

In the previous Section, we have seen an example of a ping-pong between two parties: an `initiator`, Alice,  and a `responder`, Bob.
A few questions remained unanswered there though. Namely:
- How does the Alice decide to start the interactive protocol?
- How does the Bob know which view to execute when a message from Alice comes?

A way to answer the first question is to imagine Alice connecting to her FSC node and ask the node
to instantiate a given view, to execute it, and return the generated output.

To do so, we use factories to create new instances of the view to be executed.
Here is the View Factory for the initiator's View.
```go
package pingpong

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

type InitiatorViewFactory struct{}

func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	return &Initiator{}, nil
}
```
To answer the second question, we need a way to tell the FSC node which view to execute
in response to a first message from an incoming session opened by a remote party.

## Testing

Normally, to run the `Ping Pong` sample, one would have to deploy the Fabric Smart Client nodes,
invoke the view, and so on, by using a bunch of scripts.
This is not the most convenient way to test programmatically an application.

FSC provides an `Integration Test Infrastructure` that allow the developer to:
- Describe the topology of the networks (FSC network, in this case);
- Boostrap these networks;
- Initiate interactive protocols to complete given business tasks.

Let us go step by step.

### Describe the topology of the networks

The `Ping Pong` example induces an FSC network topology with two FSC nodes.
One node for the `initiator` and another node for the `responder`.
We can describe the network topology programmatically as follows:

```go
package pingpong

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []nwo.Topology {
	// Create an empty FSC topology
	topology := fsc.NewTopology()

	// Add the initiator fsc node
	topology.AddNodeByName("initiator").SetExecutable(
		"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/cmd/initiator",
	)
	// Add the responder fsc node
	topology.AddNodeByName("responder").RegisterResponder(
		&Responder{}, &Initiator{},
	)
	return []nwo.Topology{topology}
}
```

Before we describe the meaning of the above piece of code, let us stress the following point that is crucial 
to keep in mind.
One limitation of golang is that it cannot load code at runtime. This means that all views
that a node might use must be burned inside the FSC node executable (We will solve this problem
by adding support for YAEGI, [`Issue 19`](https://github.com/hyperledger-labs/fabric-smart-client/issues/19)).

Now, let us go step by step and explain the meaning of the above code. Our goal is to describe 
the nodes in the network we want to boostrap. Each node will have its own definition.
We have two nodes here, the `initiator`, Alice, and the `responder`, Bob. 

- **Create an empty FSC network topology**: We start by creating an empty topology to which FSC node 
  definitions will be added.
- **Add the `initiator` FSC node (Executable's Package Path)**: One way to add an FSC node to the topology 
  is to use the `AddNodeByName` method.
  This method creates and returns an empty description of an FSC node and assign it a name.
  When the node description is ready, it can be populated in multiple ways.
  In the above example, the `initiator` node is populated by setting the `Executable's Package Path`.
  Indeed, the method `SetExecutable` allows the developer to specify the package path that contains the main go file.
  Here is the content of the main file:
```go
package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

func main() {
	node := fscnode.New()
	node.Execute(func() error {
		// Here, register view factories and responders as needed
		registry := view.GetRegistry(node)
		if err := registry.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
```
- **Add the responder FSC node (Executable Synthesizer)** as : Again, we start by adding a new FSC node definition 
  to the topology for the `responder`.
  However, we describe the responder node differently.
  Indeed, the node definition can be populated directly with the view factories and responders.
  Then, at network bootstrap, the integration infrastructure synthesise the responder's `main` file on the fly.
  This is the output of the synthesization:
```go
package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

func main() {
	n := fscnode.New()
	n.InstallSDK(fabric.NewSDK(n))
	n.Execute(func() error {
		// Here, register view factories and responders as needed
		registry := viewregistry.GetRegistry(n)
		registry.RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})

		return nil
	})
}
```

### Boostrap these networks

Once the topology is ready, the relative networks can be bootstrapped by creating a new integration test infrastructure.

```go
var err error
// Create the integration ii
ii, err = integration.Generate(StartPort2(), pingpong.Topology()...)
Expect(err).NotTo(HaveOccurred())
// Start the integration ii
ii.Start()
```

### Initiate interactive protocols to complete given business tasks

Now, it is time for Alice to initiate the ping pong. To do so, Alice must contact her 
FSC node and ask the node to create a new instance of the `init` view, and run it.
Now, recall that each FSC node exposes a GRPC service, the `View Service`, to do exactly this. Alice just needs to have a client to connect
to this GRPC service and send the proper command. Fortunately enough, the Integration Test Infrastructure
take care also of this.

To get the View Service client for Alice, we can do the following:
```go
alice := ii.Client("initiator")
```

We are now ready to put together all components in a BDD test.
To make everything more concrete, let us take an example and see how its BDD test looks like.
For this purpose, the ping-pong example will do the job.

```go
Describe("Network-based Ping pong", func() {
    var (
        ii *integration.Infrastructure
    )

    AfterEach(func() {
        // Stop the ii
        ii.Stop()
    })

    It("generate artifacts & successful pingpong", func() {
        var err error
        // Create the integration ii
        ii, err = integration.Generate(StartPort2(), pingpong.Topology()...)
        Expect(err).NotTo(HaveOccurred())
        // Start the integration ii
        ii.Start()
        time.Sleep(3 * time.Second)
        // Get a client for the fsc node labelled initiator
        initiator := ii.Client("initiator")
        // Initiate a view and check the output
        res, err := initiator.CallView("init", nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
    })
})
```

Let us describe what is happening in the above BDD test:

- **Create the integration network**: This steps creates the
  integration test infrastructure consisting of all networks described by the passed topologies.
  Each node in each network gets one or more network ports starting from
  initial network port number used to construct the integration infrastructure.
- **Start the integration infrastructure**: To do this, simply call the `Start` function.
  Depending on the specific networks, configuration files, crypto material, and so on
  will be generated.
  Finally, the nodes will be executed in their own process.
- **Get a client for the FSC node labelled `initiator`**:
  To access a FSC node, the test developers can get an instance of the
  View Service client by the node's name.
- **Initiate a view and check the output**:
  With the View Service client in hand, the test developers can initiate a view on the given node
  and get the result.
  
## Deeper Dive

There are still questions to answers. Here are some:
- How do I configure an FSC node?
- How does an FSC node know where are the other nodes and who they are (their PKs)?
- Where are information stored?

The above questions can be answered by looking at an FSC node's configuration file. 

### FSC node's Configuration File

Let's start from the configuration file. 
Here is the annotated configuration file used for the `initiator`, Alice. 
You can find it [`here`](./testdata/fsc/fscnodes/fsc.initiator/core.yaml) too.

````yaml
---
# Logging section
logging:
 # Spec
 spec: grpc=error:debug
 # Format
 format: '%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'
fsc:
  # The FSC id provides a name for this peer instance and is used when
  # naming docker resources.
  id: fsc.initiator
  # The networkId allows for logical separation of networks and is used when
  # naming docker resources.
  networkId: 2bhw25xuircy7mqvyxwllcnzsq
  # This represents the endpoint to other FSC nodes in the same organization.
  address: 127.0.0.1:20000
  # Whether the FSC node should programmatically determine its address
  # This case is useful for docker containers.
  # When set to true, will override FSC address.
  addressAutoDetect: true
  # GRPC Server listener address   
  listenAddress: 127.0.0.1:20000
  # Identity of this node, used to connect to other nodes
  identity:
    # X.509 certificate used as identity of this node
    cert:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/msp/signcerts/initiator.fsc.example.com-cert.pem
    # Private key matching the X.509 certificate
    key:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/msp/keystore/priv_sk
  # TLS Settings
  # (We use here the same set of properties as Hyperledger Fabric)
  tls:
    # Require server-side TLS
    enabled:  true
    # Require client certificates / mutual TLS for inbound connections.
    # Note that clients that are not configured to use a certificate will
    # fail to connect to the peer.
    clientAuthRequired: false
    # X.509 certificate used for TLS server
    cert:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/tls/server.crt
    # Private key used for TLS server
    key:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/tls/server.key
    # X.509 certificate used for TLS when making client connections.
    # If not set, fsc.tls.cert.file will be used instead
    clientCert:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/tls/server.crt
    # Private key used for TLS when making client connections.
    # If not set, fsc.tls.key.file will be used instead
    clientKey:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/tls/server.key
    # rootcert.file represents the trusted root certificate chain used for verifying certificates
    # of other nodes during outbound connections.
    rootcert:
      file: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/tls/ca.crt
    # If mutual TLS is enabled, clientRootCAs.files contains a list of additional root certificates
    # used for verifying certificates of client connections.
    clientRootCAs:
      files:
      - ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/tls/ca.crt
    rootCertFile: ./../../crypto/ca-certs.pem
  # Keepalive settings for node server and clients
  keepalive:
    # MinInterval is the minimum permitted time between client pings.
    # If clients send pings more frequently, the peer server will
    # disconnect them
    minInterval: 60s
    # Interval is the duration after which if the server does not see
    # any activity from the client it pings the client to see if it's alive
    interval: 300s
    # Timeout is the duration the server waits for a response
    # from the client after sending a ping before closing the connection
    timeout: 600s
  # P2P configuration
  p2p:
    # Listening address
    listenAddress: /ip4/127.0.0.1/tcp/20001
    # If empty, this is a P2P boostrap node. Otherwise, it contains the name of the FCS node that is a bootstrap node
    bootstrapNode: 
  # The Key-Value Store is used to store various information related to the FSC node
  kvs:
    persistence:
      # Persistence type can be `badger` (on disk) or `memory`
      type: badger
      opts:
        path: ./../../fscnodes/fsc.initiator/kvs
  # The endpoint section tells how to reach other FSC node in the network.
  # For each node, the name, the domain, the identity of the node, and its addresses must be specified.
  endpoint:
    resolves: 
    - name: initiator
      domain: fsc.example.com
      identity:
        id: initiator
        path: ./../../crypto/peerOrganizations/fsc.example.com/peers/initiator.fsc.example.com/msp/signcerts/initiator.fsc.example.com-cert.pem
      addresses:
         Listen: 127.0.0.1:20000
         P2P: 127.0.0.1:20001
         View: 127.0.0.1:20000
    - name: responder
      domain: fsc.example.com
      identity:
        id: responder
        path: ./../../crypto/peerOrganizations/fsc.example.com/peers/responder.fsc.example.com/msp/signcerts/responder.fsc.example.com-cert.pem
      addresses:
         # GRPC Server listening address
         Listen: 127.0.0.1:20002
         # P2P listening address
         P2P: 127.0.0.1:20003
         # View Service listening address (usually the same as Listen)
         View: 127.0.0.1:20002
````