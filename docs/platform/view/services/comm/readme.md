# Communication (comm) service

The Communication (Comm) layer is the backbone of the Fabric Smart Client (FSC) P2P network. It provides a reliable, secure, and multiplexed communication channel between FSC nodes, allowing Views to exchange messages seamlessly across the network.

## Overview

The Comm layer abstracts the underlying network transport and provides a session-based interface (`view.Session`). It handles:

- **Peer Discovery and Routing**: Resolving peer identities to network addresses.
- **Transport Security**: Enforcing secure handshakes and identity binding.
- **Session Multiplexing**: Running multiple logical sessions over a single physical connection (see [Multiplexer Spec](./multiplexer.md)).
- **Reliability and Flow Control**: Managing buffers and handling slow consumers.

## Architecture

### Components

1. **P2PNode**: The central orchestrator that manages active streams, sessions, and message dispatching. A single dispatcher goroutine delivers incoming messages to the sessions (see [Delivery and slow consumers](#delivery-and-slow-consumers)).
2. **EndpointService**: The source of truth for peer identities and addresses. It manages resolvers that map PeerIDs to network endpoints and provides the public keys used for dynamic trust updates.
3. **P2PHost**: An interface for transport-specific implementations. FSC supports multiple transport types (e.g., libp2p, WebSocket). The interface exposes `PeerID()`, which returns the local node's cryptographic peer identifier — the same value remote peers observe as `StreamInfo.RemotePeerID` and `SessionInfo.RemotePKID` when they connect to this node.
4. **NetworkStreamSession**: Implements the `view.Session` interface. It provides the high-level API (`Send`, `SendError`, `Receive`, `Close`) used by View developers. A session drops a stream as soon as reading from it fails, and closes when it is left without one (see [Reliability and stability](#reliability-and-stability)).

### Message flow

1. A View calls `context.GetSession(caller, party)`.
2. The Comm service queries the **EndpointService** to resolve the `party`'s network address and identity.
3. The Comm service either retrieves an existing session or creates a new one.
4. When a message is sent, the `P2PNode` checks for a cached stream to the target peer.
5. Messages are wrapped in a `ViewPacket` (Protobuf) which includes session and context metadata.
6. On the receiving side, the `dispatchMessages` loop routes incoming packets to the correct `NetworkStreamSession` based on the `SessionID` and `PeerID`.

## Transport types

FSC supports different transport types, each suited for different deployment scenarios.

### [libp2p](./libp2p.md)

A P2P stack based on the `libp2p` library, over TCP with Noise or TLS. Like WebSocket, it dials each peer at the P2P address from the endpoint resolver.

### [WebSocket](websocket.md)

A lightweight transport suitable for environments where standard HTTP/HTTPS ports are preferred. It uses standard HTTPS for handshakes and upgrades to WebSockets for communication.

## Security

Security is integrated at every level of the Comm stack:

- **Node Identity**: All transport implementations use the node's main identity (configured via `fsc.identity.key.file` and `fsc.identity.cert.file`) for securing the transport layer.
- **Transport Security**: All communication is encrypted and authenticated at the transport layer (e.g., mTLS for WebSocket, Noise/TLS for libp2p).
- **Identity Binding**: The identity asserted in the application layer is validated against the cryptographically verified identity from the transport layer. This ensures that the remote peer's identity is a verified source of truth.
- **Session Identity Exposure**: Each `NetworkStreamSession` populates `SessionInfo.RemotePKID` with the cryptographically verified remote peer ID and `SessionInfo.LocalPKID` with the local node's peer ID (obtained via `P2PHost.PeerID()`). Both fields are available to view code through `session.Info()`, enabling symmetric identity resolution and signing without out-of-band lookups.
- **Session Isolation**: All logical sessions are internally identified by a combination of the `SessionID` and the **authenticated** `PeerID` of the remote participant. This prevents attackers from injecting messages into sessions between other peers.
- **Resource Hardening**: The Comm layer enforces strict limits to prevent Denial of Service (DoS) attacks:
  - **Message Size Limit**: Incoming messages are capped by `fsc.p2p.maxRecvMsgSize` and outgoing ones by `fsc.p2p.maxSendMsgSize` (default 10 MiB each) to prevent remote memory exhaustion (OOM) attacks.
    - **Sender Behavior**: Attempts to send messages exceeding this limit will fail locally with an error (`message size … exceeds maximum send size …`), preventing network transmission.
    - **Recipient Behavior**: The recipient uses fail-fast reading by parsing the message length prefix first. If the prefix exceeds `fsc.p2p.maxRecvMsgSize`, the recipient immediately severs the physical connection and unregisters the stream to protect system resources.
    - **Payload vs Envelope**: A raw payload as large as the limit always fails, as the total message size (including metadata like `SessionID` and `ContextID`) slightly exceeds it. With the default limit, the practical maximum payload size is slightly under 10 MiB.
  - **Master Session Protection**: Delivery to the "master session" (handling unknown traffic) never blocks the dispatcher. If the master session's queue is full, the message is dropped and counted in `dropped_messages`; the master session stays open.
  - **Responders per Peer**: The view P2P service runs at most `fsc.p2p.maxRespondersPerPeer` (default 1000) responders concurrently for each authenticated remote peer. A first message over the limit gets a `busy` error reply (see [Error replies a node sends unasked](#error-replies-a-node-sends-unasked)), so the initiator fails fast. Up to the same number of rejections can be pending per peer, counted separately from the responders; further first messages are dropped. One peer can thus have at most `limit` responders plus `limit` pending rejections, and cannot starve the other peers. A slot is freed only when its responder returns, so responders that never return keep their slot until the node shuts down.
  - **Responders per Node**: `fsc.p2p.maxResponders` (default 0, unlimited) bounds the responders running concurrently across all peers, so that many peers, each within its own limit, cannot together exhaust the node. A first message from a peer under its own limit that would exceed it gets a `node_busy` error reply; a peer at its own limit gets `busy`, whatever the node-wide count. The same number separately bounds the rejections pending across all peers; further first messages are dropped. A message admitted while the node drains counts against both limits.

### Stream preamble

Every stream opens with a preamble (`platform/view/services/comm/preamble`), so that nodes of different releases learn what the other supports without a decode error or a timeout. Its message only ever gains fields: no field is renumbered, retyped or removed, and `make buf-breaking` checks this against the latest release.

- **Content**: the FSC version and VCS revision, the framing epoch, and per component the range of wire format versions the binary supports: FSC's `view` at `{1, 1}` and `fabric` and `fabricx` at `{1, 2}`, and the components the application registers (see [Format negotiation](#format-negotiation)).
- **Framing epoch**: an integer (`preamble.FramingEpoch`, 1) raised only when the framing of `ViewPacket`s on a stream changes. A node refuses a peer whose epoch differs; the error names both epochs and starts with `version:`. FSC versions and revisions are logged at debug level and never compared, so nodes of different releases keep talking to each other.
- **After authentication**: a transport reads the peer's preamble only after it has bound the peer ID to the TLS certificate or the libp2p key.
- **Size limits**: a preamble over 4 KiB, or with a string field over 256 bytes, is refused.
- **Older peers**: a peer of a release without preambles is accepted and has no preamble; WebSocket logs it once per peer. Such a peer supports version 1 of every FSC component.
- **Where nodes find it**: the transport attaches the peer's preamble to the stream (`host.PreambleStream`), and a session negotiates from the preamble of the stream it sends on.

| Transport | Client to server                                                                                                             | Server to client                                       | Refusal                                                                                                                |
| --------- | ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| WebSocket | `StreamMeta.preamble`, on every sub-connection                                                                               | `Fsc-Preamble` header (base64) on the upgrade response | a server answers the sub-connection with an Err frame and keeps the WebSocket; a client closes the WebSocket it dialed |
| libp2p    | one length-prefixed frame in each direction when a `/fsc/view/2.0.0` stream opens, within 30 seconds or the dialer's context | same                                                   | the stream is reset                                                                                                    |

On WebSocket this is the multiplexed provider, which nodes use; the test-only `SimpleProvider` neither sends nor checks a preamble. The size limits are part of the format and never shrink, so a node never refuses a preamble that an older release accepts.

A libp2p node registers `/fsc/view/2.0.0` and `/fsc/view/1.0.0` and dials `2.0.0` first. Multistream negotiation picks the first protocol the peer supports, so a peer that only knows `1.0.0` gets a stream without a preamble. A preamble that arrives and is refused, because it is malformed, too large or names another epoch, is counted in `fsc_view_services_comm_host_websocket_ws_rejected_streams{reason="version"}` (server side) and `fsc_view_services_comm_host_libp2p_rejected_streams{reason="version"}` (see [Metrics](../monitoring_metrics.md)). They are counted by the transport, not by the view P2P service: the preamble is checked before any `ViewPacket` is read, and a peer of another epoch may not read our `ViewPacket` framing.

### Format negotiation

Two nodes use, per session, the highest version of each component's formats that both support, so a node talks to peers of its own release and of the previous one at the same time.

- **Components**: every binary publishes FSC's `view`, `fabric` and `fabricx` (`preamble.FSCComponents`). An application adds its own with `preamble.RegisterComponent(name, min, max)` when its SDK is built or in `main`, before the node starts, so that `node version` and `node peers --check` report them without starting a node. A name registered twice panics, naming both ranges. Names match `[a-z0-9-]+`. The components belong to the process: every node it runs publishes the same.
- **Choosing**: an initiator session (one this node opened with `NewSession` or `NewSessionWithID`) chooses, per local component, the highest version in both ranges, from the preamble of the stream its first message goes on, and states the result in `ViewPacket.versions` (an encoded `preamble.ComponentVersions` of at most 4 KiB, refused before it is decoded) on that message and every later one it sends, so a session's versions never change and the responder learns them from whichever message reaches it first. Messages of a session the node did not open carry none. A component the peer does not list, or every component of a peer without a preamble, counts as `{1, 1}`. A component without overlap is stated as version 0, so both nodes fail on it with `view.ErrNoCommonVersion`. Two nodes that open a session with an agreed ID (`NewSessionWithID`) each negotiate and pick the same version.
- **Reading the version**: `view.SessionVersion(ctx, session, component)`, or `Version` on a session that implements `view.VersionedSession`, returns the version. On an initiator session the first call, or the first `Send`, negotiates, opening the stream if needed; a codec calls it before it encodes the first payload. A component without overlap fails with `view.ErrNoCommonVersion`, naming both ranges, before anything is sent. A session that does not implement the interface, such as an in-process one, and a responder session whose initiator stated nothing (an older release) use version 1.
- **Checking**: the responder checks each stated version of a component it has against its own range and rejects one outside it, including 0, with `no_common_version: <component> <version> not in [<min>, <max>]`, and a list that does not decode with `no_common_version: invalid versions`, before any limit or authorizer applies. This happens only if the responder's ranges changed since the initiator read them or the initiator misbehaves; a component the responder does not have is not checked.
- **Payload bytes do not change**: FSC adds and strips nothing inside `Send` and `Receive`. A component's codec reads the version and picks its encoding.
- **libp2p replicas**: on libp2p all sessions to a peer share its streams, and replicas of one node share its peer ID. A session negotiates on one stream and may send later messages on another, so replicas of different releases behind one peer ID can see a session at a version they do not support.

### Error replies a node sends unasked

A first message, one that reaches the master session, either starts a responder view or is answered with an error the initiator did not ask for. The view P2P service (`platform/view/services/view/p2p`) sends every such error through one path:

- **Reasons and payload**: The payload is `<reason>: <detail>`. `busy: too many concurrent responders for this peer` answers a peer over `fsc.p2p.maxRespondersPerPeer`. `node_busy: too many concurrent responders on this node` answers a peer under that limit while the node runs `fsc.p2p.maxResponders` responders. `draining: node is draining` answers a message that would start a new protocol on a node that drains before it stops ([shutdown.md](../shutdown.md)). `unknown_view: <view>` answers a message for which neither its protocol name nor its caller view has a responder on this node, typically because the two nodes run different versions of the application or the view moved to another package without a protocol name (see [Protocol names](../../programming-model.md#protocol-names)); `<view>` is the type path the initiator sent. `unauthorized: caller may not start this view` answers a sender the responder's authorizer refused, and `authorizer_error: authorizer failed` one it could not check (see [Authorizing responders](../../programming-model.md#authorizing-responders)). `no_common_version: <component> <version> not in [<min>, <max>]` answers a message that states a version outside this node's range (see [Format negotiation](#format-negotiation)). The detail is reduced to printable ASCII (other bytes become `?`) and at most 256 bytes.
- **The initiator fails at once**: The reply carries the initiator's session ID with status `ERROR`, so the initiator's `Receive` returns the error instead of waiting for its session to time out.
- **No loops**: A first message with status `ERROR` or an empty caller is dropped without a reply and starts no responder. Error replies are such messages, so two nodes cannot answer each other's rejections. A late message from a responder whose initiator session is gone is usually dropped this way; if the initiator sent more than one message on that session, it carries the initiator's view as caller and gets an `unknown_view` reply, which is dropped in turn.
- **Bounded cost**: Rejections share the per-peer and node-wide bounds on pending rejections described above. An authorizer's `unauthorized` or `authorizer_error` reply is the exception: it is sent from the responder slot the message already holds, so it is bounded by the responder limits instead. A peer that stops reading holds its rejection slots until its stream resets, so with `fsc.p2p.maxResponders` set it can fill the node-wide bound, and other peers' over-limit messages are dropped instead of rejected until then; their responders are unaffected. A rejection is written only to a stream to the peer that already exists (on WebSocket, the sub-connection the message arrived on; on libp2p, any live stream to that peer). It never opens a connection and never registers a session, so it cannot disturb a responder that uses a session with the same ID. If no stream is left, the rejection is dropped.
- **Typed errors on the initiator**: `session.RejectionError` (`platform/view/services/comm/session`) turns an error reply into an error whose text is `received error from remote [<payload>]`, the same for every reply. For a payload that starts with a reason above it also wraps `session.ErrRejected` and the reason's sentinel, so code can test it with `errors.Is`: `ErrUnknownView`, `ErrBusy` (`busy` and `node_busy`), `ErrDraining`, `ErrUnauthorized` or `view.ErrNoCommonVersion`; `authorizer_error` wraps only `ErrRejected`. Any other error reply, such as one a responder sends with `SendError`, wraps nothing. The receive helpers in that package (`ReadMessageWithTimeout`, `ReadFirstMessage`, `ReadFirstMessageOrPanic` and the JSON session's `Receive`) return these errors. The reason is what the remote node claims: a responder can send the same text with `SendError`, so treat it as a hint for retrying or reporting, not as proof that a check ran. The view API carries a remote view's error as text, so a client of a view that failed this way matches the text.
- **Redial after `draining`**: An initiator whose session gets a `draining:` reply retires its cached connections to that peer (`host.RetiringHost`; on WebSocket, the multiplexed WebSocket). Sessions already open on them keep working, and the next session dials again, so a load balancer in front of the peer's replicas can route it to a replica that is not draining.
- **Observability**: Rejections are logged at debug level only, since the peer controls their rate; `fsc_view_services_view_p2p_rejections{reason}` counts them (see [Metrics](../monitoring_metrics.md#p2p-service)).

## Trust and access control

The Communication layer enforces strict access control at the transport level. A remote node can only establish a connection if it is explicitly trusted by the local node.

### Trust management

FSC uses a **Certificate Authority (CA) model** and **Public Key Infrastructure (PKI)** to manage trust:

- **Static Trust**: Initial trust is established through the root CA certificates configured in the node's `core.yaml`.
- **Dynamic Trust**: The Comm layer integrates with the **EndpointService**. When the system resolves a peer's identity and retrieves its public key (e.g., from a network-wide resolver or a discovery service), that public key is automatically added to the node's trusted set for subsequent handshakes.

### Connection authorization

- **Mutual Authentication**: Both parties must present credentials (TLS certificates or Noise public keys) that are signed by a trusted CA or match a known public key in the `EndpointService`.
- **Identity Enforcement**: The transport layer rejects connections from any node that cannot prove possession of the private key corresponding to its claimed identity.
- **Revocation**: A change to a peer's trust applies to new handshakes. The WebSocket transport does not close established connections when a peer is removed (see [WebSocket](websocket.md)).

## Performance and flow control

### Concurrent message dispatching

The `P2PNode` dispatches incoming messages with a single dispatcher goroutine (`DefaultDispatcherWorkers = 1`), which keeps each session's messages in arrival order. Delivery never blocks, so a single slow or stalled session does not block the entire node (see [Delivery and slow consumers](#delivery-and-slow-consumers)).

### Atomic stream leasing

The Comm layer uses an **atomic lease mechanism** to prevent race conditions between sending messages and closing streams. When a session attempts to use a cached stream, it must successfully "lease" the stream by atomically incrementing its reference counter (`refCtr`) using compare-and-swap operations. This ensures that:

- A stream cannot be leased if it's already marked as closed or dead
- Multiple sessions can safely share the same physical stream
- On a host with per-session streams (WebSocket), the stream is closed when all leases are released (refCtr reaches 0). A host whose streams are shared by all sessions to a peer (libp2p, see [libp2p.md](libp2p.md)) keeps the stream open, because the remote may still reply on it for another session
- The first message sent over a freshly created stream is delivered reliably even if concurrent operations attempt to close it

This prevents the race condition where a message appears to send successfully but the stream is closed immediately after, causing the remote peer to receive an EOF and drop the message.

### Delivery and slow consumers

A single dispatcher goroutine delivers incoming messages to every session, which keeps each session's messages in arrival order. It never waits for a session's consumer, so a consumer that stops reading cannot hold up the others.

- **Internal Buffering**: Each session queues up to 2048 unread messages (two channels of 1024).
- **Full Queue**: A message for a full session is dropped and counted in `dropped_messages`, and the session is closed, so the consumer never receives a message that follows a dropped one. The master session is never closed this way: closing it would stop the node accepting new sessions. A session leaves the registry when it closes. Messages for a session that closed on its own, after a full queue or the loss of its streams, are dropped for a minute instead of reaching the master session, so they do not start a new responder.

## Reliability and stability

- **Race-Free State Management**: All shared metadata are protected by synchronized mutexes and atomic flags. Stream handlers use atomic operations for lifecycle management (`closed`, `dead`, `refCtr`) to prevent race conditions between concurrent send and close operations.
- **Synchronized Stream Operations**: The `streamHandler.close()` method acquires the stream lock to synchronize with concurrent `send()` operations, preventing corruption of in-flight messages.
- **Dead Stream Removal**: When reading from a stream fails, the stream is removed from every session holding it. A session left without a stream closes after a 5-second grace period unless a new stream to the same session attaches in the meantime, and leaves the registry, so a view waiting on `Receive()` gets a closed channel instead of waiting for a counterparty that is gone. The master session never closes this way. On libp2p, where all sessions to a peer share one stream, a dead stream closes every session to that peer that neither sends nor receives within the grace period.
- **Stream Lifecycle Protection**: Streams are marked as "dead" immediately upon read errors, preventing new leases while allowing existing operations to complete gracefully.
- **Deadlock Prevention**: The session shutdown process is hardened to prevent hangs. When a session is closed, it attempts to drain remaining messages to the consumer with a **500ms timeout per message**. This ensures that closing a session is never permanently blocked by an unresponsive consumer.

## Usage

### For view developers

Views typically interact with the Comm layer indirectly through the `view.Context`:

```go
// Initiate a session with a remote party
session, err := context.GetSession(caller, remoteParty)
if err != nil {
    return err
}

// Send a message
err = session.Send(context.Background(), []byte("Hello Peer"))

// Receive a message
msg := <-session.Receive()
```

### Best practices

1. **Always Close Sessions**: Explicitly calling `session.Close()` frees its streams and its registry entry immediately.
2. **Handle Receive Timeouts**: Don't wait indefinitely on `session.Receive()`. Use a `select` block with a timeout or `context.Done()`.
3. **Monitor Session Closures**: Handle session closure errors gracefully in your View logic.

## Configuration

Configuration is managed via the FSC configuration file (usually `core.yaml`). For a complete configuration reference including all available options, see the [Configuration Guide](../../../../configuration.md).

```yaml
fsc:
  p2p:
    # Transport type: "websocket" or "libp2p". There is no default -- an unset
    # or unrecognised type is rejected at startup.
    type: websocket
    listenAddress: /ip4/0.0.0.0/tcp/11511
    # Buffer size for the incoming messages channel. Default: 1024
    # This controls how many messages can be queued before blocking message dispatch.
    incomingMessagesBufferSize: 1024
    # Buffer size for stream readers. Default: 4096
    # This controls the internal buffer size used when reading protobuf messages from streams.
    streamReaderBufferSize: 4096
```

For transport-specific configuration options and detailed examples, see the [libp2p](./libp2p.md) and [WebSocket](websocket.md) documentation.

## Observability

The Comm layer exports several Prometheus metrics:

- `sessions`: Current number of active logical sessions.
- `active_streams`: Number of active physical connections.
- `opened_streams` / `closed_streams`: Counters for connection churn.
- `dropped_messages`: Counter of messages dropped due to full buffers.
