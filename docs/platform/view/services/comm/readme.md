# Communication (Comm) Service

The Communication (Comm) layer is the backbone of the Fabric Smart Client (FSC) P2P network. It provides a reliable, secure, and multiplexed communication channel between FSC nodes, allowing Views to exchange messages seamlessly across the network.

## Overview

The Comm layer abstracts the underlying network transport and provides a session-based interface (`view.Session`). It handles:
- **Peer Discovery and Routing**: Resolving peer identities to network addresses.
- **Transport Security**: Enforcing secure handshakes and identity binding.
- **Session Multiplexing**: Running multiple logical sessions over a single physical connection.
- **Reliability and Flow Control**: Managing buffers and handling slow consumers.

## Architecture

### Components

1.  **P2PNode**: The central orchestrator that manages active streams, sessions, and message dispatching. It uses a **bounded worker pool** for concurrent message dispatching to prevent resource exhaustion.
2.  **EndpointService**: The source of truth for peer identities and addresses. It manages resolvers that map PeerIDs to network endpoints and provides the public keys used for dynamic trust updates.
3.  **P2PHost**: An interface for transport-specific implementations. FSC supports multiple transport types (e.g., Libp2p, REST/WebSocket).
4.  **NetworkStreamSession**: Implements the `view.Session` interface. It provides the high-level API (`Send`, `Receive`, `Close`) used by View developers. It includes **automatic cleanup** of dead stream references to prevent memory bloat.

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

### [REST / WebSocket](./rest.md)
A lightweight transport suitable for environments where standard HTTP/HTTPS ports are preferred. It uses standard HTTPS for handshakes and upgrades to WebSockets for communication.

## Security

Security is integrated at every level of the Comm stack:

-   **Node Identity**: All transport implementations use the node's main identity (configured via `fsc.identity.key.file` and `fsc.identity.cert.file`) for securing the transport layer.
-   **Transport Security**: All communication is encrypted and authenticated at the transport layer (e.g., mTLS for REST, Noise/TLS for Libp2p).
-   **Identity Binding**: The identity asserted in the application layer is strictly validated against the cryptographically verified identity from the transport layer. This ensures that the remote peer's identity is a verified source of truth.
-   **Session Isolation**: All logical sessions are internally identified by a combination of the `SessionID` and the **authenticated** `PeerID` of the remote participant. This prevents attackers from injecting messages into sessions between other peers.
-   **Resource Hardening**: The Comm layer enforces strict limits to prevent Denial of Service (DoS) attacks:
    -   **Message Size Limit**: A global 10MB limit is enforced on all incoming messages to prevent remote memory exhaustion (OOM) attacks.
    -   **Dispatcher Bounding**: The message dispatcher uses a bounded pool of **10 workers**. This prevents goroutine explosion even if many sessions are congested.
    -   **Master Session Protection**: Delivery to the "master session" (handling unknown traffic) is capped at a **5-second timeout** to ensure worker goroutines are not permanently stalled by junk traffic.

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

### Atomic Stream Initialization
The Comm layer ensures that the first message sent over a freshly created stream is delivered reliably even if the stream is concurrently removed from the global cache (e.g., due to a rapid connection rejection by the remote peer). The initialization process atomically links the session to the new stream handler, preventing "stream not found" race conditions.

### Reliable Delivery and Backpressure
The Comm layer uses a **blocking-with-timeout** strategy to ensure reliable delivery while preventing slow consumers from deadlocking the node.

- **Internal Buffering**: Each session and stream has an internal message queue (default size: 4096).
- **Backpressure Handling**: If a consumer is slow and the internal buffer fills up, the producer will block for a maximum of **1 minute**.
- **Fail-Safe Closure**: If the buffer remains full after the timeout, the Comm layer logs an error and **closes the session/stream**.

## Reliability and Stability

- **Race-Free State Management**: All shared metadata are protected by synchronized mutexes and atomic flags.
- **Automatic Resource Pruning**: Long-lived sessions periodically prune references to closed network streams.
- **Deadlock Prevention**: The session shutdown process is hardened to prevent hangs. When a session is closed, it attempts to drain remaining messages to the consumer with a **500ms timeout per message**. This ensures that the global dispatcher (which waits for sessions to close) is never permanently blocked by an unresponsive consumer.

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
err = session.Send([]byte("Hello Peer"))

// Receive a message
msg := <-session.Receive()
```

### Best Practices
1.  **Always Close Sessions**: Explicitly calling `session.Close()` helps free resources immediately.
2.  **Handle Receive Timeouts**: Don't wait indefinitely on `session.Receive()`. Use a `select` block with a timeout or `context.Done()`.
3.  **Monitor Session Closures**: Handle session closure errors gracefully in your View logic.

## Configuration

Configuration is managed via the FSC configuration file (usually `core.yaml`):

```yaml
fsc:
  p2p:
    # Transport type: "libp2p" or "websocket"
    type: libp2p
    listenAddress: /ip4/0.0.0.0/tcp/11511
```

For transport-specific configuration options, see the [Libp2p](./libp2p.md) and [REST](./rest.md) documentation.

## Observability

The Comm layer exports several Prometheus metrics:
-   `sessions`: Current number of active logical sessions.
-   `active_streams`: Number of active physical connections.
-   `opened_streams` / `closed_streams`: Counters for connection churn.
-   `dropped_messages`: Counter of messages dropped due to full buffers.
