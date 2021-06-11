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

## Network Topology

The above example induces an FSC network topology with two FSC nodes.
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
Step by step, this is what happens:

- **Create an empty FSC network topology**: We start by creating an empty topology to which FSC node definitions will be added.
- **Add the `initiator` view node (Executable's Package Path)**: One way to add an FSC node to the topology is to use the `AddNodeByName` method.
  This method creates and returns an empty description of an FSC node and assign it a name.
  Once a node description is created, it can be populated in multiple ways.
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
		registry := view.GetRegistry(node)
		if err := registry.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
```
- **Add the responder view node (Executable Synthesizer)** as : The responder node is added to the view network
  as it has been done for the `initiator`.
  Though, the responder node is populated differently.
  Indeed, in the case of the responder, the node definition can be populated directly with the view factories
  and responders needed.
  Then, then the network is bootstrapped, the responder's main go file will be synthesize on the fly.
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
		registry := viewregistry.GetRegistry(n)
		registry.RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})

		return nil
	})
}
```


\subsection{Network}\label{subsec:network}

Once all topologies are defined, then the relative networks can be bootstrapped by
creating a new integration network.
For each view node, the integration network gives access to an instance of the
\textsf{view.Client} interface that allows the developer to initiate views on that node.
\newline

\noindent We are now ready to put together all components in a BDD test.
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
  integration network consisting of all networks described by the passed topologies.
  Each node in each network is assigned one or more network ports starting from
  initial network port number used to construct the integration network.
- **Start the integration network**: Once the network is created,
  it can be started.
  Depending on the specific network, configuration files, crypto material, and so on
  will be generated.
  Finally, the nodes will be executed in their own process.
- **Get a client for the view node labelled initiator**:
  To access a view node, the test developers can get an instance of the
  \textsf{view.Client} interface by the node's name.
- **Initiate a view and check the output**:
  With the client interface in hand, the test developers can initiate a view on the given node
  and get the result.