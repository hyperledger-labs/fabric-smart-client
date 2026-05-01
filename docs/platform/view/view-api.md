# View API

The View API is the public programming surface of the Fabric Smart Client (FSC) View platform. It is the set of contracts application code implements and consumes when defining protocols, opening sessions, composing nested views, and interacting with node services.

The public contracts are defined in the `platform/view/view` package. The runtime implementation, factory registration, and responder dispatching live in `platform/view/services/view`.

This document focuses on the public API and its runtime semantics. For the internal orchestration model, see the [View service](services/view-service.md). For the platform-level architecture, see the [View SDK architecture](view-sdk.md).

## Overview

At the API level, a view-based FSC application is built from four concepts:

- **View**: A unit of protocol logic implemented by application code.
- **Context**: The execution environment supplied to a running view.
- **Session**: A logical communication channel to another FSC node.
- **Identity**: The local or remote identity bound to a context or session.

The typical lifecycle is:

1. A caller resolves or constructs a view instance.
2. The runtime creates a `view.Context`.
3. The view executes through `Call(context)`.
4. The view uses the context to retrieve services, open sessions, invoke nested views, and manage cleanup.
5. In the standard `InitiateView` and responder execution paths, the runtime disposes the context when the protocol branch completes. In explicit `InitiateContext*` flows, the caller owns the context lifecycle and is responsible for deleting it when it is no longer needed.

## Packages

Two packages are relevant when writing and exposing views:

- `platform/view/view`: Public contracts for view code.
- `platform/view/services/view`: Runtime implementation, factory registration, and responder registration.

The first package is the programming API. The second package is the runtime that instantiates and dispatches those contracts.

## View

The core abstraction is `view.View`:

```go
type View interface {
    Call(context Context) (interface{}, error)
}
```

A view is a unit of distributed application logic, A view may:

- start a protocol with one or more remote parties;
- respond to an incoming protocol step;
- invoke other views as sub-steps of a larger protocol;
- access node services through the supplied context.

The return value is application-defined. The runtime passes it back to synchronous callers such as local execution, web `CallView`, and gRPC `CallView`. Asynchronous initiation paths, such as gRPC `InitiateView`, may return a context identifier instead of the final result.

## Context

`view.Context` is the execution environment for a running view:

```go
type Context interface {
    SpanStarter
    GetService(v interface{}) (interface{}, error)
    ID() string
    RunView(view View, opts ...RunViewOption) (interface{}, error)
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

### Service Access

`GetService(v)` retrieves a service by type from the runtime.

The runtime first checks services registered in the current view context and then falls back to the node-level service provider.

This is the standard mechanism for accessing installed node services from a view, such as identity providers, stream services, Fabric services, persistence helpers, and application-specific services.

### Execution Identity

`Me()` returns the local identity bound to the current view context.

`IsMe(id)` checks whether a given identity resolves to the local node. This is used when a protocol accepts multiple identities or aliases for the same node and needs to decide which branch to execute locally.

### Initiator and Execution Role

`Initiator()` returns the initiator view associated with the current execution branch.

For an initiator context, this is typically the outermost initiating view. For responder contexts, it is usually unset unless a nested execution explicitly switches into initiator mode through `RunView` options.

This matters because outbound sessions are scoped by the calling view identifier. In practice, an initiator typically opens sessions with:

```go
session, err := viewCtx.GetSession(viewCtx.Initiator(), party)
```

### Context Identifier

`ID()` returns the FSC context identifier for the current protocol branch.

This identifier is used internally by FSC to correlate protocol execution, responder reuse, and session metadata. It is not the same as the Go `context.Context`.

### Nested View Execution

`RunView(view, opts...)` executes another view from within the current view.

This is the main composition mechanism for larger protocols. Depending on the supplied options, the runtime may:

- reuse the same FSC context;
- create a child context;
- override the default responder session;
- override the initiator associated with the nested execution;
- replace the underlying Go `context.Context`.

Nested execution does not bypass the runtime. The same lifecycle rules, tracing, error cleanup, and session semantics still apply.

### Session Access

`GetSession(caller, party, boundToViews...)` returns a session to a remote party for the given caller view.

The runtime caches sessions by caller view identifier and remote identity. If an open session already exists, it is reused. If not, the runtime resolves the target identity to an endpoint and opens a new session.

The `caller` argument is part of the session scope. If a new session must be created and `caller` is `nil`, the runtime returns an error.

Additional views passed in `boundToViews` are registered as aliases for the same underlying session. This is useful when multiple protocol steps should share the same logical session.

`GetSessionByID(id, party)` is the explicit-session variant. It is used when the protocol already knows the logical session identifier and wants to reopen or reuse that channel directly.

`Session()` returns the default responder session. This is only meaningful for responder-style execution. For an initiator context, it usually returns `nil`.

### Go Context and Tracing

`Context()` returns the underlying Go `context.Context` associated with the current execution.

Use this when calling services that require a standard Go context for cancellation, deadlines, or trace propagation.

The context also embeds `SpanStarter`, which exposes:

```go
type SpanStarter interface {
    StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
}
```

This is used when a view needs to start a child span explicitly, for example around a receive loop or a long-running application-specific operation.

### Error Cleanup

`OnError(callback)` registers a cleanup callback for the current execution branch.

This is an error-path cleanup mechanism. The runtime invokes these callbacks when the execution returns an error or panics. They are not general-purpose finalizers and they are not guaranteed to run on successful completion.

Use `OnError` for releasing temporary resources, rolling back in-memory state, or closing application-owned resources that should only be cleaned up on failure.

## Mutable Context

Some runtime paths expose the optional `view.MutableContext` interface:

```go
type MutableContext interface {
    ResetSessions() error
    PutService(v interface{}) error
}
```

This interface is used by the runtime to inject execution-local services into a view context, most notably stream objects for gRPC and web streaming calls.

Application code should treat `MutableContext` as an optional capability. A `view.Context` does not guarantee that it also implements `MutableContext`.

## RunView Options

`RunViewOption` customizes nested execution:

```go
type RunViewOption func(*RunViewOptions) error
```

The public helpers are:

- `AsResponder(session)`: Runs the nested view with the given session as the default responder session.
- `AsInitiator()`: Marks the nested execution as an initiator branch.
- `WithViewCall(f)`: Executes an inline function instead of the `Call` method of the supplied view.
- `WithSameContext()`: Reuses the current FSC context instead of creating a child context.
- `WithContext(ctx)`: Reuses the current FSC context and replaces the underlying Go `context.Context`.

Important semantics:

- Options are applied in order.
- `WithContext(ctx)` implies `WithSameContext()`.
- If `WithViewCall(f)` is supplied, the runtime executes `f` instead of `view.Call(...)`.
- `AsInitiator()` is only meaningful when the nested execution should behave as a new initiator branch, for example when a responder temporarily starts an outbound protocol.
- In the standard child-context path, `AsInitiator()` rebinds the current default responder session under the nested initiator view. If the current context has no default responder session, the conversion fails.

## Session Model

The View API models node-to-node communication through `view.Session`:

```go
type Session interface {
    Info() SessionInfo
    Send(payload []byte) error
    SendWithContext(ctx context.Context, payload []byte) error
    SendError(payload []byte) error
    SendErrorWithContext(ctx context.Context, payload []byte) error
    Receive() <-chan *Message
    Close()
}
```

### Send and Receive

`Send` and `SendWithContext` transmit an application payload to the remote party.

`SendError` and `SendErrorWithContext` transmit an error response. These methods set the message status to `view.ERROR`, allowing the recipient to distinguish protocol failures from normal payloads.

`Receive()` returns a channel of incoming `Message` values. Views typically consume it with a `select` and an explicit timeout rather than waiting indefinitely.

`Close()` releases resources associated with the logical session.

### Message

Incoming messages are represented as:

```go
type Message struct {
    SessionID    string
    ContextID    string
    Caller       string
    FromEndpoint string
    FromPKID     []byte
    Status       int32
    Payload      []byte
    Ctx          context.Context
}
```

The `Status` field uses the constants:

- `view.OK`
- `view.ERROR`

The metadata are part of the API contract:

- `SessionID`: Logical session identifier used by the comm layer.
- `ContextID`: FSC context identifier associated with the sender.
- `Caller`: String identifier of the calling view on the remote side.
- `FromEndpoint`: Remote endpoint identifier.
- `FromPKID`: Public key identifier verified by the transport.
- `Ctx`: Go context propagated with the message, used for tracing and cancellation-aware receive handling.

`Message.Caller` is the remote view identifier. It is not the same as the remote node identity.

### SessionInfo

`Info()` returns:

```go
type SessionInfo struct {
    ID           string
    Caller       Identity
    CallerViewID string
    Endpoint     string
    EndpointPKID []byte
    Closed       bool
}
```

This metadata serves a different purpose than `Message`:

- `Caller`: Authenticated remote identity.
- `CallerViewID`: String identifier of the remote view that initiated the session.
- `Endpoint` and `EndpointPKID`: Transport-level routing and verification data.
- `Closed`: Cached session state as reported by the runtime.

The distinction between `Message.Caller` and `SessionInfo.Caller` is important:

- `Message.Caller` identifies the remote view type.
- `SessionInfo.Caller` identifies the remote node identity.

## Identity

`view.Identity` is the identity type used by the View API:

```go
type Identity = identity.Identity
```

It is an alias to the lower-level identity representation used by FSC. Views should treat it as an opaque identity value. The runtime, endpoint service, and communication layer are responsible for resolving it, comparing aliases, and binding it to authenticated peers.

## Factories and Registration

The public `platform/view/view` package defines the contracts a view implements. Exposing those views through a node uses the runtime package `platform/view/services/view`.

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

- `RegisterFactory(id, factory)`: Associates an external view identifier such as `create` or `query` with a factory.
- `RegisterResponder(responder, initiatedBy)`: Associates a responder view with an initiator view.
- `RegisterResponderWithIdentity(responder, id, initiatedBy)`: Registers the same mapping with an explicit responder identity.

Two identifier systems are involved:

- **Factory IDs** are external names used by callers to instantiate views.
- **View identifiers** are internal type-derived identifiers used by the runtime to scope sessions and responder mappings.

This distinction is important when reading logs and runtime documentation. A factory ID such as `create` is not the same thing as the internal identifier of the resulting view type.

## Execution Roles

At the API level, a view usually executes in one of two roles.

### Initiator

An initiator view starts a protocol branch and opens sessions to other parties.

Typical characteristics:

- `Initiator()` returns the initiating view.
- `Session()` is usually `nil`.
- outbound sessions are opened with `GetSession(...)`.

### Responder

A responder view is usually executed because an incoming protocol step arrived from another FSC node. The same responder-style semantics can also be introduced locally through nested execution options such as `AsResponder(session)`.

Typical characteristics:

- `Session()` returns the default responder session.
- the view usually begins by receiving or decoding input from that session;
- the runtime determines the responder through the registration map.

Views can temporarily switch role through nested execution options such as `AsResponder(session)` and `AsInitiator()`.

## Minimal Example

The `pingpong` integration example is the smallest complete reference for the View API.

The initiator view:

```go
type Initiator struct{}

func (p *Initiator) Call(viewCtx view.Context) (interface{}, error) {
    identityProvider, err := id.GetProvider(viewCtx)
    if err != nil {
        return nil, err
    }
    responder := identityProvider.Identity("responder")

    session, err := viewCtx.GetSession(viewCtx.Initiator(), responder)
    if err != nil {
        return nil, err
    }

    if err := session.Send([]byte("ping")); err != nil {
        return nil, err
    }

    select {
    case msg := <-session.Receive():
        if msg.Status == view.ERROR {
            return nil, errors.New(string(msg.Payload))
        }
        if string(msg.Payload) != "pong" {
            return nil, errors.New("expected pong")
        }
        return "OK", nil
    case <-time.After(time.Minute):
        return nil, errors.New("responder didn't pong in time")
    }
}
```

The responder view:

```go
type Responder struct{}

func (p *Responder) Call(viewCtx view.Context) (interface{}, error) {
    session := viewCtx.Session()

    select {
    case msg := <-session.Receive():
        if string(msg.Payload) != "ping" {
            if err := session.SendError([]byte("expected ping")); err != nil {
                return nil, err
            }
            return nil, errors.New("expected ping")
        }
        if err := session.Send([]byte("pong")); err != nil {
            return nil, err
        }
        return "OK", nil
    case <-time.After(5 * time.Second):
        return nil, errors.New("time out reached")
    }
}
```

This example shows the core API pattern:

- resolve an identity;
- open or retrieve a session;
- send a payload;
- receive a message;
- branch on `view.OK` or `view.ERROR`;
- return an application result.

For a full architecture-level walkthrough of how initiators, responders, contexts, and the registry interact, see the [View service](services/view-service.md).
