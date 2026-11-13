# Programming model guide

This guide explains how to build FSC applications around views, sessions,
identities, and platform services.

## What FSC applications look like

An FSC application is a Go program that embeds an FSC node, installs one or more
SDKs, registers views, and starts the node runtime.

At a high level:

1. Your application creates a node.
2. The application installs the SDKs it needs.
3. The application registers initiator and responder views.
4. A client or remote party triggers a view.
5. The view uses the runtime context to exchange messages, access services, and
   optionally interact with Fabric.

This model lets you write business protocols directly as Go code instead of
pushing all coordination into chaincode or external workflow engines.

## Core concepts

[Core concepts](../../core-concepts.md#design-key-ingredients) defines views,
identities and sessions, and the [View API](view-api.md) specifies their
contracts. This section shows how a view uses them.

### Views

A view is the unit of application logic in FSC.

A view implements a [`Call()`](../../../platform/view/view/view.go) method with
the general shape:

```go
type MyView struct{}

func (v *MyView) Call(viewCtx view.Context) (any, error) {
	// business logic
	return nil, nil
}
```

A view can act as:

- an **initiator**, which starts a protocol
- a **responder**, which reacts to a protocol started by another party
- a **child view**, executed from another view with
  [`RunView()`](services/view-service.md#view-context)

Views are intentionally application-focused. They should express the steps of a
business interaction: gather input, identify counterparties, exchange messages,
invoke platform APIs, validate results, and return an outcome.

For more detail on the runtime that creates and executes views, see
[View service](services/view-service.md).

### Context

Each running view receives a
[`view.Context`](services/view-service.md#view-context), which is the runtime
handle for interacting with the FSC platform.

The context gives access to:

- the local identity with [`Me()`](services/view-service.md#view-context)
- session management with [`Session()`](services/view-service.md#view-context)
  and [`GetSession()`](services/view-service.md#view-context)
- platform and application services with
  [`GetService()`](services/view-service.md#view-context)
- nested view execution with
  [`RunView()`](services/view-service.md#view-context)

In practice, the context is the main object your view uses to talk to the rest
of the runtime.

### Sessions

A session is a bidirectional communication channel between two FSC parties.

Sessions are how initiators and responders exchange protocol messages. The
initiator usually opens a session to a remote identity, and the responder
receives the matching session from the runtime.

Common usage pattern:

- initiator obtains a remote identity
- initiator opens a session with
  [`GetSession()`](services/view-service.md#view-context)
- initiator sends a message
- responder reads from [`Session()`](services/view-service.md#view-context)
- responder replies on the same session

### Identities

Identities represent parties in the FSC network.

A view typically does not hardcode transport endpoints. Instead, it resolves or
retrieves an identity and uses that identity when opening sessions or invoking
higher-level APIs.

The example in
[`integration/fsc/stoprestart/initiator.go`](../../../integration/fsc/stoprestart/initiator.go)
uses the identity provider to resolve the party named `bob`, then opens a
session to that identity.

### Services

FSC uses a service-oriented runtime.

Views can retrieve services from the context using
[`GetService()`](services/view-service.md#view-context). These services can be:

- FSC runtime services
- platform services from View, Fabric, or Chaguar
- application services registered by your node

This keeps business logic decoupled from concrete wiring and storage details.

For persistence-related service boundaries, see
[Runtime DB access](services/runtime-db-access.md),
[Database layout](services/database-layout.md), and
[Database drivers](services/db-driver.md).

## Application structure

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
	"log"

	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(viewsdk.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}

	node.Execute(func() error {
		registry := viewregistry.GetRegistry(node)

		// Clients start the Initiator view under the name "init".
		if err := registry.RegisterFactory("init", &InitiatorViewFactory{}); err != nil {
			return err
		}
		// Sessions opened by another node's Initiator view start a Responder view here.
		return registry.RegisterResponder(&Responder{}, &Initiator{})
	})
}
```

`Initiator`, `Responder` and `InitiatorViewFactory` are the views of the
sections below. The node runtime is provided by
[`node.New()`](../../../node/node.go), and
[`Node.Execute()`](../../../node/node.go) runs the command line; the callback
runs once the node has started.

If the application also needs Fabric capabilities, it installs the Fabric SDK in
the same startup flow, as
[Use Fabric from a view](../fabric/README.md#use-fabric-from-a-view) shows. For
Chaguar, see
[Use Chaguar from a view](../fabric-x/README.md#use-chaguar-from-a-view).

For node startup configuration, see
[View platform configuration](configuration.md) and
[Shared node configuration](../../configuration.md).

## Initiator and responder pattern

The most important programming pattern in FSC is the initiator/responder pair.

### Initiator responsibilities

An initiator view typically:

1. receives input
2. resolves the remote party
3. opens a session
4. sends one or more messages
5. waits for replies
6. validates the result
7. returns an application outcome

Simplified from
[`integration/fsc/stoprestart/initiator.go`](../../../integration/fsc/stoprestart/initiator.go),
the pattern is:

```go
type Initiator struct {
	party string // label of the party to ping, such as "bob"
}

func (i *Initiator) Call(viewCtx view.Context) (any, error) {
	identityProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, err
	}
	responder := identityProvider.Identity(i.party)

	session, err := viewCtx.GetSession(viewCtx.Initiator(), responder)
	if err != nil {
		return nil, err
	}

	if err := session.Send(viewCtx.Context(), []byte("ping")); err != nil {
		return nil, err
	}

	// Wait at most a minute for the reply, and stop at once if the view context ends.
	reply, err := commsession.ReadMessageWithContext(viewCtx.Context(), session, time.Minute)
	if err != nil {
		return nil, err
	}
	return string(reply), nil
}
```

`commsession` is `platform/view/services/comm/session`. `ReadMessageWithContext`
returns an error reply from the responder as an error.

For a more complete example with error handling, timeouts, and message
validation, see the
[stoprestart integration test](../../../integration/fsc/stoprestart/initiator.go).

This shows the essential mechanics:

- resolve a remote identity
- open a session
- send data
- receive the response

### Responder responsibilities

A responder view typically:

1. receives the session created by the initiator
2. reads the incoming message
3. performs local business logic
4. sends back a response or status

The responder pattern is:

```go
type Responder struct{}

func (*Responder) Call(viewCtx view.Context) (any, error) {
	// Wait for the first message, at most 30 seconds or until the view context ends.
	session, payload, err := commsession.ReadFirstMessage(viewCtx)
	if err != nil {
		return nil, err
	}
	if string(payload) != "ping" {
		// Tell the initiator, which otherwise waits for a reply that never comes.
		if err := session.SendError(viewCtx.Context(), []byte("expected ping")); err != nil {
			return nil, err
		}
		return nil, errors.Errorf("expected ping, got %s", payload)
	}

	if err := session.Send(viewCtx.Context(), []byte("pong")); err != nil {
		return nil, err
	}
	return "OK", nil
}
```

For a more complete example with error handling and timeouts, see the
[stoprestart integration test](../../../integration/fsc/stoprestart/responder.go).

The responder does not create the session. It consumes the session associated
with the incoming protocol request.

A message payload is input from the remote party. The transport authenticates
who sent it, as `Session().Info().Caller`, but not what the payload claims. A
responder that acts on a claim in the payload, such as an identity the caller
says it owns, must check that claim against `Caller` (see
[Binding identities a caller claims](#binding-identities-a-caller-claims)).

By default, any node that the transport authenticates can start any registered
responder. To limit a responder to the nodes it expects, register it with an
authorizer (see [Authorizing responders](#authorizing-responders)). A responder
registered under a [protocol name](#protocol-names) stays reachable when its Go
types are renamed.

For more details on initiator/responder dispatch and responder registration, see
[View service](services/view-service.md).

## View factories and registration

Views are often created through factories so that FSC can instantiate them from
external input.

A simple factory pattern looks like this:

```go
type InitiatorViewFactory struct{}

func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	return &Initiator{party: string(in)}, nil
}
```

Factories are useful when:

- a view is started remotely through an API or CLI
- the runtime needs to deserialize input
- you want a clean separation between transport input and view construction

Responder views are registered against the initiator type so the runtime knows
which responder to launch when a session arrives.

Conceptually, registration happens during node startup inside the callback
passed to [`Execute()`](../../../node/node.go).

## Build, run and call your node

An FSC application is a Go module that depends on FSC:

```bash
go get github.com/hyperledger-labs/fabric-smart-client@v0.28.0
```

Use the latest release tag. These pages describe `main`, which can contain
changes that are not released yet.

Put the `main.go` of [Application structure](#application-structure) in the
module, with the views it registers, and build it:

```bash
go build -o mynode .
```

Each node needs a `core.yaml`, its identity and TLS material, and the addresses
of the nodes it talks to. The `Initiator` above needs a second node, `bob`, that
runs the `Responder` and is listed under `fsc.endpoint.resolvers`.
`fsccli artifactsgen gen` generates all of this from a topology file, with one
directory per node under `fsc/nodes/` (see the
[FSC CLI reference](../../reference/fsc-cli.md#artifactsgen)).

The binary is a command-line tool. `node start` starts the node and keeps it
running until it receives `SIGINT` or `SIGTERM`. The node reads its `core.yaml`
from the directory that `FSCNODE_CFG_PATH` names; without it, from the current
directory, then from `/etc/hyperledger-labs/fabric-smart-client-node`:

```bash
FSCNODE_CFG_PATH=/etc/mynode ./mynode node start
```

The other commands and the exit codes are in the
[Node CLI reference](../../reference/node-cli.md), and every `core.yaml` key is
in [Shared node configuration](../../configuration.md).

A client starts a view by the name it was registered under with
`RegisterFactory`, through the remote view API. The node serves that API only
when `fsc.view.remote.enabled` is `true`, on the transports
`fsc.view.remote.backends` lists, and it accepts only commands signed by the
node's own identity or by one listed in `fsc.view.remote.clients` (see
[Remote view API access control](services/view-service.md#remote-view-api-access-control)).
The Go client in `platform/view/services/view/client` calls the `init` view of
the node above:

```go
caPEM, err := os.ReadFile("tls/ca.crt") // the CA of the node's TLS certificate
if err != nil {
	return nil, err
}
signer, err := client.NewX509SigningIdentity("client/cert.pem", "client/key.pem")
if err != nil {
	return nil, err
}
tracerProvider, err := tracing.NewProviderFromConfig(tracing.NoOp)
if err != nil {
	return nil, err
}
c, err := client.NewClient(&client.Config{
	ConnectionConfig: &grpc.ConnectionConfig{
		Address: "localhost:20000", // fsc.grpc.address
		TLS:     grpc.SecureOptions{UseTLS: true, ServerRootCAs: [][]byte{caPEM}},
	},
}, signer, tracerProvider)
if err != nil {
	return nil, err
}
ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
defer cancel()
res, err := c.CallViewWithContext(ctx, "init", []byte("bob"))
```

If the node requires client certificates (`clientAuthRequired`), also set
`Certificate`, `Key` and `RequireClientCert` in `grpc.SecureOptions`.

`grpc` is `platform/common/services/grpc` and `tracing` is
`platform/view/services/tracing`.
[View service over HTTP](services/view-service-http.md) describes the HTTP
transport of the same client, and `fsccli view` calls a view from the shell (see
the [FSC CLI reference](../../reference/fsc-cli.md#view)).

## Running child views

Large business protocols are easier to maintain if they are split into smaller
views.

A parent view can call another view using
[`RunView()`](services/view-service.md#view-context). This is useful when you
want to separate:

- identity lookup
- validation
- data collection
- transaction assembly
- finality waiting
- persistence updates

A good rule is that each view should represent a meaningful step in a business
protocol, not just a random helper function.

## Accessing runtime and application services

For reusable application logic, prefer services over duplicating wiring inside
every view.

Patterns that work well:

- expose application state through a service
- retrieve the service from the context
- keep SQL and backend details behind the service boundary
- let the service use FSC persistence or Fabric APIs internally

This aligns with the service-oriented model described in
[`view-service.md`](services/view-service.md).

## Where Fabric fits

The View platform provides the programming model for distributed protocols. The
Fabric platform adds ledger-facing capabilities on top of that model.

In practical terms:

- use the View platform to coordinate parties
- use the Fabric platform when the protocol must read state, endorse, submit, or
  observe transaction finality

The architecture summary in [`core-concepts.md`](../../core-concepts.md)
describes this split: the View platform handles orchestration, while the Fabric
platform provides Fabric-aware APIs such as state, vault, transaction, and
finality services.

## Recommended design style

When writing FSC applications, prefer the following style:

### Keep views business-oriented

A view should describe a business interaction, not low-level infrastructure
mechanics.

Good examples:

- collect an approval from another party
- exchange and validate transaction parameters
- submit a transaction and wait for finality
- update local application state

Less ideal examples:

- embed raw database code in every view
- spread identity resolution logic across unrelated files
- mix transport, persistence, and domain logic in a single huge
  [`Call()`](../../../platform/view/view/view.go)

### Use services for reusable logic

If multiple views need the same logic, move it into a service and retrieve it
with [`GetService()`](services/view-service.md#view-context).

### Prefer explicit protocol steps

Protocols are easier to debug when the steps are clear:

- who starts
- who responds
- what message is sent
- what is validated
- what happens on timeout or error

### Treat identities as first-class inputs

Views should work in terms of FSC identities and business roles rather than raw
addresses.

## Error handling guidelines

Distributed protocols fail in real systems, so views should handle:

- session creation errors
- message send and receive failures
- timeouts
- invalid or unexpected replies
- downstream Fabric errors

A view waiting for a reply must also stop when its view context ends, which
happens when the caller cancels the view or its deadline expires. A view that
waits only on a timer keeps running until the timer fires, and so does the
responder on the other node, which learns that the protocol is over only when
the view closes its sessions. Here's an example of timeout handling with message
validation:

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
case <-viewCtx.Context().Done():
	return nil, viewCtx.Context().Err()
case <-time.After(1 * time.Minute):
	return nil, errors.New("timeout waiting for response")
}
```

`commsession.ReadMessageWithContext(viewCtx.Context(), session, timeout)` from
`platform/view/services/comm/session` does the same and returns the payload.
`commsession.ReadFirstMessage` also stops when the view context ends.

For complete examples, see:

- [`integration/fsc/pingpong`](../../../integration/fsc/pingpong/): source of a
  ping/pong protocol with timeout and validation
- [signedpingpong](../../../integration/fsc/signedpingpong) - Symmetric message
  signing using `SessionInfo.LocalPKID` and `SessionInfo.RemotePKID`
- [stoprestart](../../../integration/fsc/stoprestart) - Session handling across
  a responder restart
- [IOU](../../../integration/fabric/common/iou) - Complex Fabric integration
  with state management and endorsement collection

## Symmetric signing with session identity

In protocols where each party must sign its message and verify the counterpart's
signature, both identities — local and remote — can be resolved directly from
the session without additional parameters or out-of-band lookups.

`SessionInfo` exposes two complementary fields for this:

- `RemotePKID`: the public key identifier of the **remote** peer,
  cryptographically bound by the transport layer.
- `LocalPKID`: the public key identifier of the **local** node, the same value
  the remote peer sees as its `RemotePKID`.

A view uses the endpoint service to map either PKID to a full `view.Identity`,
then retrieves a signer or verifier from the sig service:

```go
endpointSvc := endpoint.GetService(viewCtx)
sigSvc, err := sig.GetService(viewCtx)

// Resolve own identity and sign the outgoing payload.
localID, err := endpointSvc.GetIdentity("", session.Info().LocalPKID)
signer, err := sigSvc.GetSigner(localID)
signature, err := signer.Sign(payload)

// Resolve the remote identity and verify the incoming signature.
remoteID, err := endpointSvc.GetIdentity("", session.Info().RemotePKID)
verifier, err := sigSvc.GetVerifier(remoteID)
err = verifier.Verify(payload, signature)
```

Both sides of the protocol apply the same pattern — the initiator's `LocalPKID`
equals the responder's `RemotePKID` for the same session, and vice versa. This
makes the logic symmetric and avoids hardcoding identity references in either
view.

For a complete end-to-end example of this pattern with negative-case checks, see
[`integration/fsc/signedpingpong`](../../../integration/fsc/signedpingpong).

## Suggested development flow

A practical way to build an FSC application is:

1. define the business protocol in terms of initiator and responder roles
2. model each role as a view
3. decide what data must be exchanged over sessions
4. move reusable logic into services
5. add Fabric integration only where ledger interaction is needed
6. register views during node startup
7. test the protocol end to end with integration tests

## Advanced topics

These sections cover the details of responder authorization, protocol names,
format versions and claimed identities.

### Authorizing responders

By default, any node that the transport authenticates can start any registered
responder. A responder registered with an authorizer runs only for the nodes the
authorizer accepts:

```go
import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// allowed holds the canonical node identities that may start the responder.
var allowed map[string]bool

err := view.GetRegistry(sp).RegisterResponderWithOptions(
	&Responder{}, &Initiator{},
	view.WithAuthorizer(view.AuthorizerFunc(func(ctx context.Context, caller viewapi.Identity) error {
		if !allowed[string(id.CanonicalIdentity(caller))] {
			return errors.Wrapf(view.ErrDenied, "node [%s]", caller)
		}
		return nil
	})),
)
```

`caller` is the FSC node identity the transport authenticated, the same identity
the responder later sees as `Session().Info().Caller`. The P2P service calls the
authorizer for every first message of the protocol, before it creates the view
context or session, with a context that times out after 5 seconds. The
authorizer must return once that context is done: the P2P service does not
abandon a call that ignores it. An error wrapping `view.ErrDenied` refuses the
caller with the reply `unauthorized: caller may not start this view`; any other
error, a timeout, a panic, or a sender whose identity the node cannot resolve
gives `authorizer_error: authorizer failed`. The authorizer's own error text
never reaches the caller. A running check holds a responder slot of the caller,
counted against both `fsc.p2p.maxRespondersPerPeer` and `fsc.p2p.maxResponders`,
so keep it fast.

The authorizer decides who may start the responder. It does not check the
payload: the responder must still validate every message it receives, as
[Responder responsibilities](#responder-responsibilities) describes.

Each initiator of a responder has its own authorizer, so a responder registered
for several initiators needs an authorizer on each of them: an initiator
registered without one lets any node start the responder. A later registration
for the same initiator without `WithAuthorizer`, for example a default
registration by a library, keeps the authorizer; one with `WithAuthorizer`
replaces it.

### Protocol names

A peer finds a responder by the Go type path of the initiator view
(`view.GetIdentifier`), so moving or renaming an initiator type breaks peers
that run the previous release. A protocol with a stable name does not depend on
the type path. The initiator view declares the name by implementing
`view.ProtocolView`, and the responder registers for it with `WithProtocol`:

```go
// Protocol implements view.ProtocolView.
func (*Initiator) Protocol() (component, name string) { return "iou", "create" }

err := view.GetRegistry(sp).RegisterResponderWithOptions(
	&Responder{}, &Initiator{}, view.WithProtocol("iou", "create"),
)
```

A session opened with `GetSession(caller, party)` states the name `caller`
declares on its messages. FSC's bundled views (endorsement, finality, recipient
exchange) pass `viewCtx.Initiator()` as caller, so they open sessions under the
application's protocol. The responder node looks the name up first, then the
type path, so an initiator that declares no name, or one of an older release,
still reaches a responder registered for its type. Both parts of the name match
`[a-z0-9-]+`. Names and type paths are kept apart: a type path that happens to
look like a name is never found as one. A name belongs to one initiator:
registering it for another initiator fails. A later registration for the same
initiator without `WithProtocol` keeps the name and its authorizer; one with
another name releases the old name.

### Versions of a protocol's formats

Nodes of different releases use, per session, the highest version of each
component's formats that both support (see
[Format negotiation](services/comm/readme.md#format-negotiation)). An
application that changes the format of its own messages registers a component
with the range it supports, when its SDK is built or in `main`, before the node
starts:

```go
preamble.RegisterComponent("iou", 1, 2) // platform/view/services/comm/preamble
```

Registering before the node starts lets `node version` and `node peers --check`,
which start no node, report the component too. A name registered twice,
including one of FSC's (`view`, `fabric`, `fabricx`), panics.

A view reads the version of its session before it encodes a message, and the
receiving side reads it before it decodes:

```go
v, err := viewapi.SessionVersion(viewCtx.Context(), session, "iou")
if err != nil {
	return nil, err // no version both nodes support
}
```

The version holds for the whole session. A session that is not an FSC network
session, such as an in-process one or an application's wrapper that hides the
FSC session, uses version 1.

### Binding identities a caller claims

A responder binds a remote identity in the endpoint service only after the
caller has proved it controls the identity. The FSC recipient views do this with
a signed proof. Applications with their own recipient exchange can reuse the
same proof: `state.SignRecipientProof` and `state.VerifyRecipientProof`
(`platform/fabric/services/state/proof.go`) sign and check it, and
`endpoint.Service.BindProven` binds the verified identity to the caller's node
identity. See
[Recipient proof of ownership](../fabric/security.md#recipient-proof-of-ownership).

## Related documentation

- [View API](view-api.md)
- [View SDK architecture](view-sdk.md)
- [View service](services/view-service.md)
- [View platform configuration](configuration.md)
- [Fabric platform](../fabric/README.md) and
  [Chaguar platform](../fabric-x/README.md)
- [Fabric SDK architecture](../fabric/fabric-sdk.md)
- [Core concepts](../../core-concepts.md)
