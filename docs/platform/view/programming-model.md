# Programming Model Guide

This guide explains how to build FSC applications around views, sessions, identities, and platform services.

## What FSC Applications Look Like

An FSC application is a Go program that embeds an FSC node, installs one or more SDKs, registers views, and starts the node runtime.

At a high level:

1. Your application creates a node.
2. The application installs the SDKs it needs.
3. The application registers initiator and responder views.
4. A client or remote party triggers a view.
5. The view uses the runtime context to exchange messages, access services, and optionally interact with Fabric.

This model lets you write business protocols directly as Go code instead of pushing all coordination into chaincode or external workflow engines.

## Core Concepts

### Views

A view is the unit of application logic in FSC.

A view implements a [`Call()`](../../../platform/view/view/view.go#L13) method with the general shape:

```go
type MyView struct{}

func (v *MyView) Call(viewCtx view.Context) (interface{}, error) {
	// business logic
	return nil, nil
}
```

A view can act as:

- an **initiator**, which starts a protocol
- a **responder**, which reacts to a protocol started by another party
- a **child view**, executed from another view with [`RunView()`](services/view-service.md#view-context)

Views are intentionally application-focused. They should express the steps of a business interaction: gather input, identify counterparties, exchange messages, invoke platform APIs, validate results, and return an outcome.

For more detail on the runtime that creates and executes views, see [View service](services/view-service.md).

### Context

Each running view receives a [`view.Context`](services/view-service.md#view-context), which is the runtime handle for interacting with the FSC platform.

The context gives access to:

- the local identity with [`Me()`](services/view-service.md#view-context)
- session management with [`Session()`](services/view-service.md#view-context) and [`GetSession()`](services/view-service.md#view-context)
- platform and application services with [`GetService()`](services/view-service.md#view-context)
- nested view execution with [`RunView()`](services/view-service.md#view-context)

In practice, the context is the main object your view uses to talk to the rest of the runtime.

### Sessions

A session is a bidirectional communication channel between two FSC parties.

Sessions are how initiators and responders exchange protocol messages. The initiator usually opens a session to a remote identity, and the responder receives the matching session from the runtime.

Common usage pattern:

- initiator obtains a remote identity
- initiator opens a session with [`GetSession()`](services/view-service.md#view-context)
- initiator sends a message
- responder reads from [`Session()`](services/view-service.md#view-context)
- responder replies on the same session

### Identities

Identities represent parties in the FSC network.

A view typically does not hardcode transport endpoints. Instead, it resolves or retrieves an identity and uses that identity when opening sessions or invoking higher-level APIs.

The example in [`integration/fabric/stoprestart/initiator.go`](../../../integration/fabric/stoprestart/initiator.go) uses the identity provider to resolve the party named `bob`, then opens a session to that identity.

### Services

FSC uses a service-oriented runtime.

Views can retrieve services from the context using [`GetService()`](services/view-service.md#view-context). These services can be:

- FSC runtime services
- platform services from View, Fabric, or Fabric-x
- application services registered by your node

This keeps business logic decoupled from concrete wiring and storage details.

For persistence-related service boundaries, see [Runtime DB access](services/runtime-db-access.md) and [Database drivers](services/db-driver.md).

## Application Structure

A typical FSC application has these pieces:

- a node entrypoint
- SDK installation
- view registration
- initiator views
- responder views
- optional application services
- optional Fabric integration

At startup, the application creates a node and installs SDKs.

A minimal node structure looks like this:

```go
package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(viewsdk.NewSDK(node)); err != nil {
		panic(err)
	}

	node.Execute(func() error {
		// Get the view registry
		registry := view.GetRegistry(node)
		
		// Register an initiator view factory
		if err := registry.RegisterFactory("myInitiator", &MyInitiatorViewFactory{}); err != nil {
			return err
		}
		
		// Register a responder for the initiator
		initiatorID := registry.GetIdentifier(&MyInitiator{})
		if err := registry.RegisterResponder(&MyResponder{}, initiatorID); err != nil {
			return err
		}
		
		return nil
	})
}
```

The node runtime is provided by [`node.New()`](../../../node/node.go#L45) and started through [`Node.Execute()`](../../../node/node.go#L72).

If the application also needs Fabric capabilities, it installs the Fabric SDK in the same startup flow, as described in [Fabric SDK architecture](../fabric/fabric-sdk.md). For Fabric-X capabilities, see [Fabric-X](../fabric-x/README.md).

For node startup configuration, see [View platform configuration](configuration.md) and [Shared node configuration](../../configuration.md).

## Initiator and Responder Pattern

The most important programming pattern in FSC is the initiator/responder pair.

### Initiator Responsibilities

An initiator view typically:

1. receives input
2. resolves the remote party
3. opens a session
4. sends one or more messages
5. waits for replies
6. validates the result
7. returns an application outcome

From [`integration/fabric/stoprestart/initiator.go`](integration/fabric/stoprestart/initiator.go), the pattern is:

```go
type Initiator struct {
	in []byte
}

func (p *Initiator) Call(viewCtx view.Context) (interface{}, error) {
	identityProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, err
	}
	responder := identityProvider.Identity("bob")

	session, err := viewCtx.GetSession(viewCtx.Initiator(), responder)
	if err != nil {
		return nil, err
	}

	if err := session.Send(p.in); err != nil {
		return nil, err
	}

	msg := <-session.Receive()
	return string(msg.Payload), nil
}
```

For a more complete example with error handling, timeouts, and message validation, see the [stoprestart integration test](../../../integration/fabric/stoprestart/initiator.go).

This shows the essential mechanics:

- resolve a remote identity
- open a session
- send data
- receive the response

### Responder Responsibilities

A responder view typically:

1. receives the session created by the initiator
2. reads the incoming message
3. performs local business logic
4. sends back a response or status

The responder pattern is:

```go
type Responder struct{}

func (p *Responder) Call(viewCtx view.Context) (interface{}, error) {
	session := viewCtx.Session()
	msg := <-session.Receive()

	if err := session.Send(msg.Payload); err != nil {
		return nil, err
	}
	return "OK", nil
}
```

For a more complete example with error handling and timeouts, see the [stoprestart integration test](../../../integration/fabric/stoprestart/responder.go).

The responder does not create the session. It consumes the session associated with the incoming protocol request.

For more details on initiator/responder dispatch and responder registration, see [View service](services/view-service.md).

## View Factories and Registration

Views are often created through factories so that FSC can instantiate them from external input.

A simple factory pattern looks like this:

```go
type InitiatorViewFactory struct{}

func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	return &Initiator{in: in}, nil
}
```

Factories are useful when:

- a view is started remotely through an API or CLI
- the runtime needs to deserialize input
- you want a clean separation between transport input and view construction

Responder views are registered against the initiator type so the runtime knows which responder to launch when a session arrives.

Conceptually, registration happens during node startup inside the callback passed to [`Execute()`](../../../node/node.go#L72).

## Running Child Views

Large business protocols are easier to maintain if they are split into smaller views.

A parent view can call another view using [`RunView()`](services/view-service.md#view-context). This is useful when you want to separate:

- identity lookup
- validation
- data collection
- transaction assembly
- finality waiting
- persistence updates

A good rule is that each view should represent a meaningful step in a business protocol, not just a random helper function.

## Accessing Runtime and Application Services

For reusable application logic, prefer services over duplicating wiring inside every view.

Patterns that work well:

- expose application state through a service
- retrieve the service from the context
- keep SQL and backend details behind the service boundary
- let the service use FSC persistence or Fabric APIs internally

This aligns with the service-oriented model described in [`view-service.md`](services/view-service.md).

## Where Fabric Fits

The View platform provides the programming model for distributed protocols. The Fabric platform adds ledger-facing capabilities on top of that model.

In practical terms:

- use the View platform to coordinate parties
- use the Fabric platform when the protocol must read state, endorse, submit, or observe transaction finality

The architecture summary in [`core-concepts.md`](../../core-concepts.md) describes this split:
the View platform handles orchestration, while the Fabric platform provides Fabric-aware APIs such as state, vault, transaction, and finality services.

## Recommended Design Style

When writing FSC applications, prefer the following style:

### Keep views business-oriented

A view should describe a business interaction, not low-level infrastructure mechanics.

Good examples:

- collect an approval from another party
- exchange and validate transaction parameters
- submit a transaction and wait for finality
- update local application state

Less ideal examples:

- embed raw database code in every view
- spread identity resolution logic across unrelated files
- mix transport, persistence, and domain logic in a single huge [`Call()`](../../../platform/view/view/view.go)

### Use services for reusable logic

If multiple views need the same logic, move it into a service and retrieve it with [`GetService()`](platform/view/services/view-service.md:72).

### Prefer explicit protocol steps

Protocols are easier to debug when the steps are clear:

- who starts
- who responds
- what message is sent
- what is validated
- what happens on timeout or error

### Treat identities as first-class inputs

Views should work in terms of FSC identities and business roles rather than raw addresses.

## Error Handling Guidelines

Distributed protocols fail in real systems, so views should handle:

- session creation errors
- message send and receive failures
- timeouts
- invalid or unexpected replies
- downstream Fabric errors

Here's an example of timeout handling with message validation:

```go
ch := session.Receive()
select {
case msg := <-ch:
	if msg.Status == view.ERROR {
		return nil, errors.New(string(msg.Payload))
	}
	if string(msg.Payload) != "expected_value" {
		return nil, errors.Errorf("expected expected_value, got %s", string(msg.Payload))
	}
case <-time.After(1 * time.Minute):
	return nil, errors.New("timeout waiting for response")
}
```

For complete examples, see:
- [pingpong](../../../integration/fsc/pingpong) - Simple ping/pong protocol with timeout and validation
- [stoprestart](../../../integration/fabric/stoprestart) - Session handling with stop/restart scenarios
- [IOU](../../../integration/fabric/iou) - Complex Fabric integration with state management and endorsement collection

## Suggested Development Flow

A practical way to build an FSC application is:

1. define the business protocol in terms of initiator and responder roles
2. model each role as a view
3. decide what data must be exchanged over sessions
4. move reusable logic into services
5. add Fabric integration only where ledger interaction is needed
6. register views during node startup
7. test the protocol end to end with integration tests

## Related Documentation

- [View SDK architecture](view-sdk.md)
- [View service](services/view-service.md)
- [View platform configuration](configuration.md)
- [Fabric SDK architecture](../fabric/fabric-sdk.md)
- [Core concepts](../../core-concepts.md)