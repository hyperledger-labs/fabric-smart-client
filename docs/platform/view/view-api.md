# View API

The View API is the public programming surface of the Fabric Smart Client (FSC)
View platform. It is the set of contracts application code implements and
consumes when defining protocols, opening sessions, composing nested views, and
interacting with node services.

The public contracts are defined in the `platform/view/view` package. The
runtime implementation, factory registration, and responder dispatching live in
`platform/view/services/view`.

This document focuses on the public API and its runtime semantics. For the
internal orchestration model, see the [View service](services/view-service.md).
For the platform-level architecture, see the
[View SDK architecture](view-sdk.md).

## Overview

At the API level, a view-based FSC application is built from four concepts:
view, context, session and identity.
[Core concepts](../../core-concepts.md#design-key-ingredients) defines views,
sessions and identities, and the
[Programming model guide](programming-model.md#core-concepts) shows how an
application uses them. This page specifies their contracts.

The typical lifecycle is:

1. A caller resolves or constructs a view instance.
2. The runtime creates a `view.Context`.
3. The view executes through `Call(context)`.
4. The view uses the context to retrieve services, open sessions, invoke nested
   views, and manage cleanup.
5. In the standard `InitiateView` and responder execution paths, the runtime
   disposes the context when the protocol branch completes.

## Packages

Two packages are relevant when writing and exposing views:

- `platform/view/view`: Public contracts for view code.
- `platform/view/services/view`: Runtime implementation, factory registration,
  and responder registration.

The first package is the programming API. The second package is the runtime that
instantiates and dispatches those contracts.

## View

The core abstraction is `view.View`:

```go
type View interface {
    Call(context Context) (any, error)
}
```

A view is a unit of distributed application logic. A view may:

- start a protocol with one or more remote parties;
- respond to an incoming protocol step;
- invoke other views as sub-steps of a larger protocol;
- access node services through the supplied context.

The return value is application-defined. The runtime passes it back to the
caller: local execution, and `CallView` through the view service, over gRPC and
HTTP alike.

The gRPC client sends `CallView`, `CallViewWithContext`, and `StreamCallView`
once and never retries them, because views are not idempotent and a retry could
run the same business process twice. If the client cannot connect to the node,
the call fails before the request is sent and the view does not run. If the call
fails after the request is sent, for example with `codes.Unavailable` or
`codes.DeadlineExceeded`, the outcome is unknown: the view may have run fully or
partially. The caller decides whether running it again is safe.

## Context

`view.Context` is the execution environment for a running view:

```go
type Context interface {
    SpanStarter
    GetService(v any) (any, error)
    ID() string
    RunView(view View, opts ...RunViewOption) (any, error)
    Me() Identity
    IsMe(id Identity) bool
    Initiator() View
    GetSession(caller View, party Identity, boundToViews ...View) (Session, error)
    GetSessionByID(id string, party Identity) (Session, error)
    Session() Session
    Context() context.Context
    OnError(callback func())
}
```

### Service access

`GetService(v)` retrieves a service by type from the runtime.

The runtime first checks services registered in the current view context and
then falls back to the node-level service provider.

This is the standard mechanism for accessing installed node services from a
view, such as identity providers, stream services, Fabric services, persistence
helpers, and application-specific services.

### Execution identity

`Me()` returns the local identity bound to the current view context.

`IsMe(id)` checks whether a given identity resolves to the local node. This is
used when a protocol accepts multiple identities or aliases for the same node
and needs to decide which branch to execute locally.

### Initiator and execution role

`Initiator()` returns the initiator view associated with the current execution
branch.

For an initiator context, this is typically the outermost initiating view. For
responder contexts, it is usually unset unless a nested execution explicitly
switches into initiator mode through `RunView` options.

This matters because outbound sessions are scoped by the calling view
identifier. In practice, an initiator typically opens sessions with:

```go
session, err := viewCtx.GetSession(viewCtx.Initiator(), party)
```

### Context identifier

`ID()` returns the FSC context identifier for the current protocol branch.

This identifier is used internally by FSC to correlate protocol execution,
responder reuse, and session metadata. It is not the same as the Go
`context.Context`.

### Nested view execution

`RunView(view, opts...)` executes another view from within the current view.

This is the main composition mechanism for larger protocols. Depending on the
supplied options, the runtime may:

- reuse the same FSC context;
- create a child context;
- override the default responder session;
- override the initiator associated with the nested execution;
- replace the underlying Go `context.Context`.

Nested execution does not bypass the runtime. The same lifecycle rules, tracing,
error cleanup, and session semantics still apply.

### Session access

`GetSession(caller, party, boundToViews...)` returns a session to a remote party
for the given caller view.

The runtime caches sessions by caller view identifier and remote identity. If an
open session already exists, it is reused. If not, the runtime resolves the
target identity to an endpoint and opens a new session.

The `caller` argument is part of the session scope. If a new session must be
created and `caller` is `nil`, the runtime returns an error.

Additional views passed in `boundToViews` are registered as aliases for the same
underlying session. This is useful when multiple protocol steps should share the
same logical session.

`GetSessionByID(id, party)` is the explicit-session variant. It is used when the
protocol already knows the logical session identifier and wants to reopen or
reuse that channel directly.

`Session()` returns the default responder session. This is only meaningful for
responder-style execution. For an initiator context, it usually returns `nil`.

### Go context and tracing

`Context()` returns the underlying Go `context.Context` associated with the
current execution.

Use this when calling services that require a standard Go context for
cancellation, deadlines, or trace propagation.

The context also embeds `SpanStarter`, which exposes:

```go
type SpanStarter interface {
    StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
}
```

This is used when a view needs to start a child span explicitly, for example
around a receive loop or a long-running application-specific operation.

### Error cleanup

`OnError(callback)` registers a cleanup callback for the current execution
branch.

This is an error-path cleanup mechanism. The runtime invokes these callbacks
when the execution returns an error or panics. They are not general-purpose
finalizers and they are not guaranteed to run on successful completion.

Use `OnError` for releasing temporary resources, rolling back in-memory state,
or closing application-owned resources that should only be cleaned up on
failure.

## Mutable context

Some runtime paths expose the optional `view.MutableContext` interface:

```go
type MutableContext interface {
    ResetSessions() error
    PutService(v any) error
}
```

This interface is used by the runtime to inject execution-local services into a
view context, most notably stream objects for gRPC and web streaming calls.

Application code should treat `MutableContext` as an optional capability. A
`view.Context` does not guarantee that it also implements `MutableContext`.

## RunView options

`RunViewOption` customizes nested execution:

```go
type RunViewOption func(*RunViewOptions) error
```

The public helpers are:

- `AsResponder(session)`: Runs the nested view with the given session as the
  default responder session.
- `AsInitiator()`: Marks the nested execution as an initiator branch.
- `WithViewCall(f)`: Executes an inline function instead of the `Call` method of
  the supplied view.
- `WithSameContext()`: Reuses the current FSC context instead of creating a
  child context.
- `WithContext(ctx)`: Reuses the current FSC context and replaces the underlying
  Go `context.Context`.

Important semantics:

- Options are applied in order.
- `WithContext(ctx)` implies `WithSameContext()`.
- If `WithViewCall(f)` is supplied, the runtime executes `f` instead of
  `view.Call(...)`.
- `AsInitiator()` is only meaningful when the nested execution should behave as
  a new initiator branch, for example when a responder temporarily starts an
  outbound protocol.
- In the standard child-context path, `AsInitiator()` rebinds the current
  default responder session under the nested initiator view. If the current
  context has no default responder session, the conversion fails.

## Session model

The View API models node-to-node communication through `view.Session`:

```go
type Session interface {
    Info() SessionInfo
    Send(ctx context.Context, payload []byte) error
    SendError(ctx context.Context, payload []byte) error
    Receive() <-chan *Message
    Close()
}
```

### Send and receive

`Send` transmits an application payload to the remote party using the provided
context.

`SendError` transmits an error response using the provided context. This method
sets the message status to `view.ERROR`, allowing the recipient to distinguish
protocol failures from normal payloads.

The payload passed to `SendError` is transmitted as-is. When a responder view
returns an error from `Call`, the P2P runtime forwards `err.Error()` verbatim to
the remote caller via `SendError` — it does not inspect, wrap, or redact it. The
runtime's job is only to decide _that_ an error gets returned; deciding _what_
information that error is allowed to contain is the application's
responsibility. Since the remote party may be untrusted, responder views should
return business-logic errors verbatim (callers legitimately need that detail to
react, and tests rely on it), but should wrap or replace errors that could carry
internal detail (stack-adjacent context, file paths, credentials) with a
sanitized message before returning them.

`Receive()` returns a channel of incoming `Message` values. Views typically
consume it with a `select` and an explicit timeout rather than waiting
indefinitely.

`Close()` releases resources associated with the logical session.

### Message

Incoming messages are represented as:

[`view.Message`](../../../platform/view/view/session.go) carries the payload in
`Payload`, the outcome in `Status`, and the metadata listed below.

The `Status` field uses the constants:

- `view.OK` with value `200`
- `view.ERROR` with value `500`

The metadata are part of the API contract:

- `SessionID`: Logical session identifier used by the comm layer.
- `ContextID`: FSC context identifier associated with the sender.
- `Caller`: String identifier of the calling view on the remote side.
- `FromEndpoint`: Remote endpoint identifier.
- `FromPKID`: Public key identifier verified by the transport.
- `Ctx`: Go context propagated with the message, used for tracing and
  cancellation-aware receive handling.
- `Protocol`: Stable protocol name `<component>/<name>` that the session's
  initiator states; empty if its view declares none or the message comes from a
  responder.
- `Versions`: Encoded version of each component's formats that the session's
  initiator chose, stated on every message the initiator sends; `nil` on a
  responder's messages.

`Message.Caller` is the remote view identifier. It is not the same as the remote
node identity.

### SessionInfo

`Info()` returns:

```go
type SessionInfo struct {
    ID             string
    Caller         Identity
    CallerViewID   string
    RemoteEndpoint string
    RemotePKID     []byte
    LocalPKID      []byte
    Closed         bool
}
```

This metadata serves a different purpose than `Message`:

- `Caller`: Authenticated remote identity.
- `CallerViewID`: String identifier of the remote view that initiated the
  session.
- `RemoteEndpoint` and `RemotePKID`: Transport-level routing and verification
  data for the remote peer.
- `LocalPKID`: The public key identifier of the local node — the local-side
  counterpart of `RemotePKID`. It is the identifier of the key-pair that
  instantiated the local communication stack. Views can use it to resolve the
  local identity (for example to retrieve a signer) without additional lookups,
  symmetrically to how `RemotePKID` identifies the remote peer.
- `Closed`: Cached session state as reported by the runtime.

The distinction between `Message.Caller` and `SessionInfo.Caller` is important:

- `Message.Caller` identifies the remote view type.
- `SessionInfo.Caller` identifies the remote node identity.

## Identity

`view.Identity` is the identity type used by the View API:

```go
type Identity = identity.Identity
```

It is an alias to the lower-level identity representation used by FSC. Views
should treat it as an opaque identity value. The runtime, endpoint service, and
communication layer are responsible for resolving it, comparing aliases, and
binding it to authenticated peers.

## Factories and registration

The public `platform/view/view` package defines the contracts a view implements.
Exposing those views through a node uses the runtime package
`platform/view/services/view`.

The runtime factory contract is:

```go
type Factory interface {
    NewView(in []byte) (view.View, error)
}
```

Factories are used to construct view instances from external calls such as:

- local CLI invocations;
- gRPC view calls;
- web view calls.

The runtime registration methods include:

- `RegisterFactory(id, factory)`: Associates an external view identifier such as
  `create` or `query` with a factory.
- `RegisterResponder(responder, initiatedBy)`: Associates a responder view with
  an initiator view.
- `RegisterResponderWithIdentity(responder, id, initiatedBy)`: Registers the
  same mapping with an explicit responder identity.
- `RegisterResponderWithOptions(responder, initiatedBy, opts...)`: Registers the
  mapping with options: `WithIdentity(id)`; `WithAuthorizer(a)`, which decides
  which remote nodes may start the responder (see
  [Authorizing responders](programming-model.md#authorizing-responders)); and
  `WithProtocol(component, name)`, which also registers the responder under a
  stable protocol name (see
  [Protocol names](programming-model.md#protocol-names)).

Two identifier systems are involved:

- **Factory IDs** are external names used by callers to instantiate views.
- **View identifiers** are internal type-derived identifiers used by the runtime
  to scope sessions and responder mappings.

This distinction is important when reading logs and runtime documentation. A
factory ID such as `create` is not the same thing as the internal identifier of
the resulting view type.

## Execution roles

At the API level, a view usually executes in one of two roles.

### Initiator

An initiator view starts a protocol branch and opens sessions to other parties.

Typical characteristics:

- `Initiator()` returns the initiating view.
- `Session()` is usually `nil`.
- outbound sessions are opened with `GetSession(...)`.

### Responder

A responder view is usually executed because an incoming protocol step arrived
from another FSC node. The same responder-style semantics can also be introduced
locally through nested execution options such as `AsResponder(session)`.

Typical characteristics:

- `Session()` returns the default responder session.
- the view usually begins by receiving or decoding input from that session;
- the runtime determines the responder through the registration map.

Views can temporarily switch role through nested execution options such as
`AsResponder(session)` and `AsInitiator()`.

## Example

The
[Programming model guide](programming-model.md#initiator-and-responder-pattern)
has a complete ping/pong initiator and responder that use this API. An initiator
resolves an identity, opens a session with
`GetSession(viewCtx.Initiator(), party)`, sends with `Send`, and reads the reply
from `Receive()`, checking `Message.Status` for `view.ERROR`. A responder reads
from `Session()` and replies on the same session with `Send` or `SendError`.

[`integration/fsc/signedpingpong`](../../../integration/fsc/signedpingpong)
extends the pattern with message signing based on `SessionInfo.LocalPKID` and
`SessionInfo.RemotePKID` (see
[Symmetric signing with session identity](programming-model.md#symmetric-signing-with-session-identity)).

For a walkthrough of how initiators, responders, contexts and the registry
interact at runtime, see the [View service](services/view-service.md).
