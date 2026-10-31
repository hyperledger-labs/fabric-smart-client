# View Service over HTTP

The view service is reachable on two transports: gRPC on the `fsc.grpc` listener and HTTP on
the `fsc.web` listener. The HTTP binding carries the same messages as the gRPC service
(`ViewService` in `platform/view/services/view/grpc/server/protos`), encoded as
[protojson](https://protobuf.dev/programming-guides/json/), and its handlers call the same
`protos.ViewServiceServer` instance that serves gRPC. Authentication, authorization, the binding to
the client TLS certificate, replay protection, the view timeout and response signing are
therefore one implementation for both transports, and a command accepted on one transport is a
replay on the other.

## Go client

`platform/view/services/view/client` speaks both transports with the same API, results and
errors; `Config.Transport` selects one (`TransportGRPC` when empty):

```go
c, err := client.NewClient(&client.Config{
	ConnectionConfig: &grpc.ConnectionConfig{Address: "node:20002", TLS: tlsOpts},
	Transport:        client.TransportHTTP,
}, signer, tracerProvider)
res, err := c.CallView("myView", input)
```

The client signs every command and accepts a response only if its header names the hash of the
command it sent. It presents its TLS keypair only when `TLS.RequireClientCert` is set, on both
transports.

## Endpoints

| Method | Path | Messages |
| --- | --- | --- |
| `POST` | `/v1/ViewService/ProcessCommand` | request body `SignedCommand`, response body `SignedCommandResponse` |
| `GET` (WebSocket) | `/v1/ViewService/StreamCommand` | text frames, see [Streams](#streams) |

The paths are `protos.ProcessCommandHTTPPath` and `protos.StreamCommandHTTPPath`. Bodies and
frames are protojson; `bytes` fields are base64 strings. Both ends ignore unknown fields, as gRPC
does, so nodes and clients of different versions interoperate when a message gains a field. The
`response` field of a `SignedCommandResponse` holds the binary encoding of a `CommandResponse`,
exactly as on gRPC, so a client decodes and checks it the same way on both transports.

## Errors

Every request whose body decodes as a `SignedCommand` gets `200` and a `SignedCommandResponse`.
Failures of the command itself, such as an unknown signer, a stale or replayed command, a view
error or a view timeout, are reported in its `CommandResponse.err`, as on gRPC.

Other statuses mean no command reached the view service:

| Status | Cause |
| --- | --- |
| `400` | the body is not a protojson `SignedCommand` |
| `405` | a method other than `POST` |
| `409` | the node's shutdown cancelled the view (gRPC `Aborted`); it may have had effects, so check its outcome rather than retrying ([shutdown.md](shutdown.md)) |
| `413` | the body is larger than `fsc.web.maxRecvMsgSize` (100 MiB when unset) |
| `500` | the view service returned an error instead of a response; the body does not carry it |
| `503` | the node drains before it stops (gRPC `Unavailable`); the command did not run, so retry on another replica ([shutdown.md](shutdown.md)) |

## Streams

A stream is a WebSocket whose text frames carry the messages of the gRPC stream, in the same
order:

1. the client sends one `SignedCommand`;
2. the view and the client exchange `CallViewResponse` messages, in both directions, for as long as
   the view runs;
3. the server sends one final `SignedCommandResponse` and closes the connection with code 1000.

When the view service ends the stream with an error instead of a final response, the server closes
with code 1011, or with code 1013 (try again later) when the node drains before it stops; a view the
node's shutdown cancelled closes with 1011 and a reason that says so ([shutdown.md](shutdown.md)). A frame larger than `fsc.web.maxRecvMsgSize` (100 MiB when unset) ends the stream. Browsers may open a stream only from
the listener's own origin.

## Configuration

The HTTP binding is served when `fsc.view.remote.enabled` is true and `fsc.view.remote.backends`
lists `web`, on the `fsc.web` listener. See `fsc.view.remote` in the
[configuration reference](../../../configuration.md).

## Transport security

The view routes do not require a client certificate: the signed command authenticates the caller.
The listener's `fsc.web.tls` settings still apply to the TLS handshake, with the same three
client-authentication states as on `fsc.grpc`. When the listener verified a client certificate, the
command's `tlsCertHash` must name it.
