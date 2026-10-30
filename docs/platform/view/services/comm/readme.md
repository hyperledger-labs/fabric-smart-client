# Communication (Comm) Service

The Communication (Comm) layer is the backbone of the Fabric Smart Client (FSC) P2P network. It provides a reliable, secure, and multiplexed communication channel between FSC nodes, allowing Views to exchange messages seamlessly across the network.

## Overview

The Comm layer abstracts the underlying network transport and provides a session-based interface (`view.Session`). It handles:
- **Peer Discovery and Routing**: Resolving peer identities to network addresses.
- **Transport Security**: Enforcing secure handshakes and identity binding.
- **Session Multiplexing**: Running multiple logical sessions over a single physical connection (see [Multiplexer Spec](./multiplexer.md)).
- **Reliability and Flow Control**: Managing buffers and handling slow consumers.

## Architecture

### Components

1.  **P2PNode**: The central orchestrator that manages active streams, sessions, and message dispatching. It uses a **bounded worker pool** for concurrent message dispatching to prevent resource exhaustion.
2.  **EndpointService**: The source of truth for peer identities and addresses. It manages resolvers that map PeerIDs to network endpoints and provides the public keys used for dynamic trust updates.
3.  **P2PHost**: An interface for transport-specific implementations. FSC supports multiple transport types (e.g., Libp2p, WebSocket). The interface exposes `PeerID()`, which returns the local node's cryptographic peer identifier — the same value remote peers observe as `StreamInfo.RemotePeerID` and `SessionInfo.RemotePKID` when they connect to this node.
4.  **NetworkStreamSession**: Implements the `view.Session` interface. It provides the high-level API (`Send`, `SendError`, `Receive`, `Close`) used by View developers. A session drops a stream as soon as reading from it fails, and closes when it is left without one (see [Reliability and Stability](#reliability-and-stability)).

### Message Flow

1.  A View calls `context.GetSession(party)`.
2.  The Comm service queries the **EndpointService** to resolve the `party`'s network address and identity.
3.  The Comm service either retrieves an existing session or creates a new one.
4.  When a message is sent, the `P2PNode` checks for a cached stream to the target peer.
5.  Messages are wrapped in a `ViewPacket` (Protobuf) which includes session and context metadata.
6.  On the receiving side, the `dispatchMessages` loop routes incoming packets to the correct `NetworkStreamSession` based on the `SessionID` and `PeerID`.

## Transport Types

FSC supports different transport types, each suited for different deployment scenarios.

### [Libp2p](./libp2p.md)
A robust, decentralized P2P stack based on the industry-standard `libp2p` library. It is recommended for decentralized deployments where peer discovery and NAT traversal are required.

### [WebSocket](websocket.md)
A lightweight transport suitable for environments where standard HTTP/HTTPS ports are preferred. It uses standard HTTPS for handshakes and upgrades to WebSockets for communication.

## Security

Security is integrated at every level of the Comm stack:

-   **Node Identity**: All transport implementations use the node's main identity (configured via `fsc.identity.key.file` and `fsc.identity.cert.file`) for securing the transport layer.
-   **Transport Security**: All communication is encrypted and authenticated at the transport layer (e.g., mTLS for Websocket, Noise/TLS for Libp2p).
-   **Identity Binding**: The identity asserted in the application layer is strictly validated against the cryptographically verified identity from the transport layer. This ensures that the remote peer's identity is a verified source of truth.
-   **Session Identity Exposure**: Each `NetworkStreamSession` populates `SessionInfo.RemotePKID` with the cryptographically verified remote peer ID and `SessionInfo.LocalPKID` with the local node's peer ID (obtained via `P2PHost.PeerID()`). Both fields are available to view code through `session.Info()`, enabling symmetric identity resolution and signing without out-of-band lookups.
-   **Session Isolation**: All logical sessions are internally identified by a combination of the `SessionID` and the **authenticated** `PeerID` of the remote participant. This prevents attackers from injecting messages into sessions between other peers.
-   **Resource Hardening**: The Comm layer enforces strict limits to prevent Denial of Service (DoS) attacks:
    -   **Message Size Limit**: A global 10MB limit is enforced on all incoming and outgoing messages to prevent remote memory exhaustion (OOM) attacks.
        -   **Sender Behavior**: Attempts to send messages exceeding this limit will fail locally with an error (`message header or payload too large`), preventing network transmission.
        -   **Recipient Behavior**: The recipient uses fail-fast reading by parsing the message length prefix first. If the prefix exceeds 10MB, the recipient immediately severs the physical connection and unregisters the stream to protect system resources.
        -   **Payload vs Envelope**: Note that a 10MB raw payload will always fail, as the total message size (including metadata like `SessionID` and `ContextID`) will slightly exceed the limit. The practical maximum payload size is approximately 9.9MB.
    -   **Master Session Protection**: Delivery to the "master session" (handling unknown traffic) is capped at a **5-second timeout** to ensure worker goroutines are not permanently stalled by junk traffic.
    -   **Responders per Peer**: The view P2P service runs at most `fsc.p2p.maxRespondersPerPeer` (default 1000) responders concurrently for each authenticated remote peer. A first message over the limit gets a `busy` error reply (see [Error replies a node sends unasked](#error-replies-a-node-sends-unasked)), so the initiator fails fast. Up to the same number of rejections can be pending per peer, counted separately from the responders; further first messages are dropped. One peer can thus have at most `limit` responders plus `limit` pending rejections, and cannot starve the other peers. A slot is freed only when its responder returns, so responders that never return keep their slot until the node shuts down.

### Error replies a node sends unasked

A first message, one that reaches the master session, either starts a responder view or is answered with an error the initiator did not ask for. The view P2P service (`platform/view/services/view/p2p`) sends every such error through one path:

-   **Reasons and payload**: The payload is `<reason>: <detail>`. `busy: too many concurrent responders for this peer` answers a peer over `fsc.p2p.maxRespondersPerPeer`. `unknown_view: <view>` answers a message whose caller view has no responder on this node, typically because the two nodes run different versions of the application or the view moved to another package; `<view>` is the identifier the initiator sent. The detail is reduced to printable ASCII (other bytes become `?`) and at most 256 bytes.
-   **The initiator fails at once**: The reply carries the initiator's session ID with status `ERROR`, so the initiator's `Receive` returns the error instead of waiting for its session to time out.
-   **No loops**: A first message with status `ERROR` or an empty caller is dropped without a reply and starts no responder. Error replies are such messages, so two nodes cannot answer each other's rejections. A late message from a responder whose initiator session is gone is usually dropped this way; if the initiator sent more than one message on that session, it carries the initiator's view as caller and gets an `unknown_view` reply, which is dropped in turn.
-   **Bounded cost**: Rejections share the per-peer bound on pending rejections described above. A rejection is written only to a stream to the peer that already exists (on WebSocket, the sub-connection the message arrived on; on libp2p, any live stream to that peer). It never opens a connection and never registers a session, so it cannot disturb a responder that uses a session with the same ID. If no stream is left, the rejection is dropped.
-   **Observability**: Rejections are logged at debug level only, since the peer controls their rate; `fsc_view_services_view_p2p_rejections{reason}` counts them (see [Metrics](../monitoring_metrics.md#p2p-service)).

## Trust and Access Control

The Communication layer enforces strict access control at the transport level. A remote node can only establish a connection if it is explicitly trusted by the local node.

### Trust Management
FSC uses a **Certificate Authority (CA) model** and **Public Key Infrastructure (PKI)** to manage trust:
- **Static Trust**: Initial trust is established through the root CA certificates configured in the node's `core.yaml`.
- **Dynamic Trust**: The Comm layer integrates with the **EndpointService**. When the system resolves a peer's identity and retrieves its public key (e.g., from a network-wide resolver or a discovery service), that public key is automatically added to the node's trusted set for subsequent handshakes.

### Connection Authorization
- **Mutual Authentication**: Both parties must present credentials (TLS certificates or Noise public keys) that are signed by a trusted CA or match a known public key in the `EndpointService`.
- **Identity Enforcement**: The transport layer rejects connections from any node that cannot prove possession of the private key corresponding to its claimed identity.
- **Revocation**: Connections are immediately closed if a peer's trust status changes or if the underlying transport security is compromised.

## Performance and Flow Control

### Concurrent Message Dispatching
The `P2PNode` uses a **bounded worker pool** to dispatch incoming messages. This prevents goroutine explosion and ensures that a single slow or stalled session does not block the entire node.

### Atomic Stream Leasing
The Comm layer uses an **atomic lease mechanism** to prevent race conditions between sending messages and closing streams. When a session attempts to use a cached stream, it must successfully "lease" the stream by atomically incrementing its reference counter (`refCtr`) using compare-and-swap operations. This ensures that:
- A stream cannot be leased if it's already marked as closed or dead
- Multiple sessions can safely share the same physical stream
- On a host with per-session streams (websocket), the stream is closed when all leases are released (refCtr reaches 0). A host whose streams are shared by all sessions to a peer (libp2p, see [libp2p.md](libp2p.md)) keeps the stream open, because the remote may still reply on it for another session
- The first message sent over a freshly created stream is delivered reliably even if concurrent operations attempt to close it

This prevents the race condition where a message appears to send successfully but the stream is closed immediately after, causing the remote peer to receive an EOF and drop the message.

### Delivery and Slow Consumers
A single dispatcher goroutine delivers incoming messages to every session, which keeps each session's messages in arrival order. It never waits for a session's consumer, so a consumer that stops reading cannot hold up the others.

- **Internal Buffering**: Each session queues up to 2048 unread messages (two channels of 1024).
- **Full Queue**: A message for a full session is dropped and counted in `dropped_messages`, and the session is closed, so the consumer never receives a message that follows a dropped one. The master session is never closed this way: closing it would stop the node accepting new sessions. A session leaves the registry when it closes. Messages for a session that closed on its own, after a full queue or the loss of its streams, are dropped for a minute instead of reaching the master session, so they do not start a new responder.

## Reliability and Stability

- **Race-Free State Management**: All shared metadata are protected by synchronized mutexes and atomic flags. Stream handlers use atomic operations for lifecycle management (`closed`, `dead`, `refCtr`) to prevent race conditions between concurrent send and close operations.
- **Synchronized Stream Operations**: The `streamHandler.close()` method acquires the stream lock to synchronize with concurrent `send()` operations, preventing corruption of in-flight messages.
- **Dead Stream Removal**: When reading from a stream fails, the stream is removed from every session holding it. A session left without a stream closes after a 5-second grace period unless a new stream to the same session attaches in the meantime, and leaves the registry, so a view waiting on `Receive()` gets a closed channel instead of waiting for a counterparty that is gone. The master session never closes this way. On libp2p, where all sessions to a peer share one stream, a dead stream closes every session to that peer that neither sends nor receives within the grace period.
- **Stream Lifecycle Protection**: Streams are marked as "dead" immediately upon read errors, preventing new leases while allowing existing operations to complete gracefully.
- **Deadlock Prevention**: The session shutdown process is hardened to prevent hangs. When a session is closed, it attempts to drain remaining messages to the consumer with a **500ms timeout per message**. This ensures that closing a session is never permanently blocked by an unresponsive consumer.

## Usage

### For View Developers
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

### Best Practices
1.  **Always Close Sessions**: Explicitly calling `session.Close()` frees its streams and its registry entry immediately.
2.  **Handle Receive Timeouts**: Don't wait indefinitely on `session.Receive()`. Use a `select` block with a timeout or `context.Done()`.
3.  **Monitor Session Closures**: Handle session closure errors gracefully in your View logic.

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

For transport-specific configuration options and detailed examples, see the [Libp2p](./libp2p.md) and [WekSocket](websocket.md) documentation.

## Observability

The Comm layer exports several Prometheus metrics:
-   `sessions`: Current number of active logical sessions.
-   `active_streams`: Number of active physical connections.
-   `opened_streams` / `closed_streams`: Counters for connection churn.
-   `dropped_messages`: Counter of messages dropped due to full buffers.
