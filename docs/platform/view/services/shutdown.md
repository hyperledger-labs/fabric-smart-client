# Graceful shutdown

A node that gets SIGTERM or SIGINT, or whose `Node.Stop` is called, lets running views finish
before it stops its components, and stops them in an order where none outlives what it uses.
`Node.Stop` and `node start` therefore block for up to `fsc.shutdown.drainTimeout` plus
`fsc.shutdown.teardownTimeout`.

## Order

1. The node drains and stops taking new work:
   - `GET /readyz` answers 503 with `{"status":"not ready","reason":"draining"}`, so a load
     balancer takes the node out of rotation. `GET /healthz` stays 200.
   - The gRPC health service reports `NOT_SERVING` for every service.
   - The gauge `fsc_shutdown_draining` is 1.
   - A new view-API command is refused (see [View-API clients](#view-api-clients)).
   - A first P2P message for a new protocol is refused (see [P2P peers](#p2p-peers)).
2. Views already running get up to `fsc.shutdown.drainTimeout` to finish. The node
   logs every 5s that it is still waiting. A view counts as running from the moment
   `InitiateView` (or the view service) starts it, and a responder from the moment its first
   message is admitted, until it returns. Only the view API and P2P refuse new work: a view
   the application starts itself with `InitiateView` during the drain still runs and is
   waited for. Goroutines a view starts itself, such as with
   `view.RunView`, are not waited for.
3. At the timeout, every initiator view still running is cancelled with
   `view.ErrCancelledByShutdown`, and the node waits at most one more second for them to
   return. It does not wait for a view that ignores cancellation.
4. The node context is cancelled, and `Stop()` is called on every SDK in reverse
   install order, for at most `fsc.shutdown.teardownTimeout` in total. With the usual
   `fabricsdk.NewFrom(viewsdk.NewSDK(...))` this stops the Fabric networks (delivery and
   committer) first, then the view SDK's web, metrics, gRPC and operations servers, then the
   comm service, then the KVS. An SDK still stopping at the timeout is logged and left running.

A second SIGTERM or SIGINT, or a second `Node.Stop` call, skips the rest of the drain and goes
on with the teardown.

```yaml
fsc:
  shutdown:
    # How long running views may take to finish. Set it at least as long as the longest view,
    # typically the finality timeout. 0 does not wait; unset or negative is 30s.
    drainTimeout: 30s
    # How long the SDKs may take to stop. 0 does not wait; unset or negative is 30s.
    teardownTimeout: 30s
```

## View-API clients

On gRPC and on the HTTP binding ([view-service-http.md](view-service-http.md)):

| Situation | gRPC | HTTP `ProcessCommand` | WebSocket `StreamCommand` | What to do |
|-----------|------|-----------------------|---------------------------|------------|
| The node drains | `Unavailable` "node is draining" | 503 | close code 1013 (try again later) | Retry on another replica. The command did not run. |
| A shutdown cancelled the view | `Aborted` "view cancelled by node shutdown" | 409 | close code 1011, the reason naming the shutdown | Check the outcome. The view may have had effects, such as a submitted transaction that still commits; do not retry it blindly. |

The Go client (`platform/view/services/view/client`) reports both as gRPC status errors on
either transport, so `status.Code(err)` tells them apart.

## P2P peers

While draining, a node answers a first message for a responder it hosts with the error
`draining: node is draining`, so the initiator fails at once. A message for a protocol already
in progress is still admitted: one whose context ID names a responder context that a session of
the same peer (the same `FromPKID`) already joined. The per-peer responder and rejection limits
still apply.

When an initiator gets `draining: ` on a session it opened, its comm layer retires its cached
WebSocket connections to that peer. Sessions already open on them keep working; the next session
to that peer dials a new connection, which a load balancer can route to another replica.

## Replicas behind a load balancer

Configure the balancer's health check on `GET /readyz` of the `fsc.web` listener. A draining
replica turns unready at once, so new connections go to the other replicas, while its open
connections keep serving the protocols they carry until they finish.

## Kubernetes

Give the pod time for the whole sequence:

```
terminationGracePeriodSeconds >= preStop sleep + drainTimeout + teardownTimeout + 5s
```

That is 70s with a 5s `preStop` sleep and the default timeouts. The `preStop` sleep gives the
endpoints controller time to take the pod out of the Service before the node starts refusing
work.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fsc-node
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 70
      containers:
      - name: fsc-node
        image: example/fsc-node:latest
        args: ["node", "start"]
        lifecycle:
          preStop:
            exec:
              command: ["sleep", "5"]
        startupProbe:
          httpGet: {path: /healthz, port: web, scheme: HTTPS}
          failureThreshold: 30
          periodSeconds: 2
        livenessProbe:
          httpGet: {path: /healthz, port: web, scheme: HTTPS}
          periodSeconds: 10
        readinessProbe:
          httpGet: {path: /readyz, port: web, scheme: HTTPS}
          periodSeconds: 2
        ports:
        - {name: web, containerPort: 9000}
        - {name: p2p, containerPort: 9001}
```

With `tls.clientAuthRequired: true` on `fsc.web`, a probe without a client certificate fails the
TLS handshake; use a TCP probe or an exec probe that presents one.

## SDK authors

`pkg/node` drains through the `node.Drainer` it finds in the node's registry; the view SDK
registers the view manager, which implements it. An SDK does not need to implement or pass on
anything for the drain. To release what it started, an SDK implements `Stop() error` (part of
`commondig.SDK`) and calls the `Stop` of the SDK it wraps last, so that what it uses outlives it.
