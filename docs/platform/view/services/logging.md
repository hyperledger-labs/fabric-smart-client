# Logging

This page is for people who run an FSC node or embed one in an application. It
explains what a node logs, how to choose what it logs while it runs, and how to
collect the lines. The `logging` keys of `core.yaml` and their defaults are in
[Node configuration](../../../configuration.md). Developers writing FSC code
find the rules for log calls in
[Conventions → Logging & monitoring](../../../dev/conventions.md#logging--monitoring).

## What a node writes

A node writes one line per entry to standard error, in the format of
`logging.format`. `logging.spec` sets the level of each logger; when it is
empty, the node reads the `FABRIC_LOGGING_SPEC` environment variable, and uses
`info` when that is unset too. An invalid format or spec stops the node at
startup.

At startup the node logs the active spec and handler:

```text
2026-11-26 20:15:01.627 CET 0001 INFO [fsc.platform.common.services.logging] Init -> logging spec [grpc=error:libp2p=warn:info], handler [default]
```

This is an Info line of the logger `fsc.platform.common.services.logging`, so a
spec that sets that logger, or the default, to Warn hides it.

## Logger names

Every line carries the name of the logger that wrote it. FSC's names are `fsc.`
followed by the package path with dots for slashes: the package
`platform/view/services/comm` logs as `fsc.platform.view.services.comm`. Some
packages add a suffix, so one package can have several loggers below its name.

A node also has these names:

- `panurus.` and the package path below `token/`, when Panurus runs in the node.
- `grpc` for the gRPC library and `libp2p` for the libp2p comm driver.
- The short names of the Fabric libraries FSC links. They depend on the
  platforms the node runs:
  - every node: `flogging.httpadmin`, the `/logspec` handler;
  - with the Fabric platform, also `bccsp`, `bccsp_sw` and `protoutil`;
  - with the Chaguar platform, also `msp`, `msp.identity`, `policies`,
    `policy`, `cauthdsl`, `common.channelconfig`, `common.configtx`,
    `common.capabilities` and `aclmgmt`, from fabric-x-common.

These prefixes are a good start when you look into one part of a node:

| Prefix                                       | What it covers                       |
| -------------------------------------------- | ------------------------------------ |
| `fsc.pkg.node`                               | node start and stop                  |
| `fsc.platform.view.sdk.dig`                  | the gRPC, web and operations servers |
| `fsc.platform.view.services.comm`            | sessions and messages between nodes  |
| `fsc.platform.view.services.view`            | views                                |
| `fsc.platform.view.services.storage`         | databases                            |
| `fsc.platform.fabric.core.generic.committer` | Fabric transaction commit            |
| `fsc.platform.fabric.core.generic.delivery`  | Fabric block delivery                |
| `fsc.platform.fabric.core.generic.ordering`  | Fabric ordering service              |
| `fsc.platform.fabricx`                       | the Chaguar platform                 |

## The logging spec

A spec is a list of entries separated by `:`. Each entry is a level, or one or
more logger names separated by `,` followed by `=` and a level. An entry
without a name sets the level of every logger no other entry matches. The
levels FSC uses are `debug`, `info`, `warn` and `error`, in lower or upper case
(`debug` or `DEBUG`, not `Debug`); `warning` means `warn`.

An entry applies to the logger it names and to the loggers below it, matching
whole dot-separated segments from the start of the name. When several entries
match, the longest name wins. A name that ends with `.` matches that logger
only, not the ones below it.

So `fsc.platform.fabricx=debug` turns on Debug for the whole Chaguar platform,
and `fabricx=debug` matches no logger at all. On a Chaguar node, `msp=debug`
changes only fabric-x-common's MSP: FSC's own MSP code logs under
`fsc.platform.fabric.core.generic.msp` and `fsc.platform.fabric.core.msp`.

The specs FSC ships quiet gRPC and libp2p, which log routine events at Info:

```yaml
logging:
  spec: grpc=error:libp2p=warn:info
```

To follow the commits of a Fabric node, add an entry for the committer:

```yaml
logging:
  spec: grpc=error:libp2p=warn:fsc.platform.fabric.core.generic.committer=debug:info
```

## Changing levels at runtime

The operations endpoint `/logspec` reads and replaces the spec of a running
node:

- `GET /logspec` returns `{"spec":"..."}` with the active spec. Entries with a
  name come first, sorted, and the default level last.
- `PUT /logspec` with the body `{"spec":"..."}` activates that spec for every
  logger, including loggers created before the call, and returns 204.
- A body that isn't valid JSON, an invalid spec or any other method returns 400
  with `{"error":"..."}` and changes nothing.

A PUT replaces the whole spec; it doesn't add to it. A body of `{}` or an empty
spec resets every logger to Info, including `grpc` and `libp2p`, so send the
full spec you want. A spec set this way lasts until the node restarts, which
reads `logging.spec` again.

The endpoint is served on the `fsc.web` listener, or on a listener of its own
when `fsc.metrics.address` is set. With `fsc.web.enabled: false` and no
`fsc.metrics.address`, no listener serves it.

A request needs a client certificate the listener verifies, unless
`fsc.metrics.clientAuthRequired` is false; without one it gets 401. The comment
on that key in [Node configuration](../../../configuration.md) says what
happens on a listener that never asks for a certificate.

With the node's TLS files:

```bash
curl --cacert tls/ca.crt --cert tls/client.crt --key tls/client.key \
  https://node.example.com:20002/logspec
curl --cacert tls/ca.crt --cert tls/client.crt --key tls/client.key \
  -X PUT -d '{"spec":"grpc=error:libp2p=warn:fsc.platform.view.services.comm=debug:info"}' \
  https://node.example.com:20002/logspec
```

## Log levels

FSC picks the level of a line by what an operator should see at the default
level, Info:

- **Debug**: details of each transaction, message, session or query. Off in
  production.
- **Info**: lifecycle events, such as the node starting or stopping, a channel
  becoming ready, a connection established, or a configuration change. A node
  at Info in steady state writes no line per transaction.
- **Warn**: something went wrong and the node handled it, such as a lost
  connection, a retry or a dropped message. Rare in a healthy node.
- **Error**: something failed, nothing recovered it, and no caller got the
  error. Look into it.

The rules for developers are in
[Conventions → Log levels](../../../dev/conventions.md#log-levels).

## Output formats

### Pattern

Any `logging.format` other than `json` is a pattern, as in Fabric. Its verbs are
`%{color}`, `%{id}`, `%{level}`, `%{message}`, `%{module}` (the logger name),
`%{shortfunc}` and `%{time}`; `%{time}` takes a Go time layout, as in
`%{time:15:04:05.000}`, and `%{level:.4s}` shortens the level to four letters.
`%{color}` writes ANSI escape codes even when standard error is not a terminal,
so leave it out of a format whose output goes to a file or a collector. An
empty format is Fabric's default:

```text
%{color}%{time:2006-01-02 15:04:05.000 MST} %{id:04x} %{level:.4s}%{color:reset} [%{module}] %{color:bold}%{shortfunc}%{color:reset} -> %{message}
```

### JSON

With `logging.format: json`, each line is one JSON object:

```json
{"level":"info","ts":1795719847.0405,"name":"fsc.platform.common.services.logging","caller":"logging/config.go:91","msg":"logging spec [info], handler [default]"}
```

- `level`: `debug`, `info`, `warn` or `error`.
- `ts`: the time in seconds since the Unix epoch, with fractions.
- `name`: the logger name.
- `caller`: the file and line of the log call.
- `msg`: the message.
- `stacktrace`: on Error lines, the stack of the log call, as one string with
  newlines inside the JSON value. The line stays a single line.
- The fields the code adds with `With` and the `*w` methods, the context fields
  of `*Context` lines, and `trace_id` and `span_id` (see
  [Trace IDs in log lines](#trace-ids-in-log-lines)).

## Collecting logs

Set `logging.format: json` on every node whose logs you collect. Grafana Alloy,
Fluent Bit, Vector and the OpenTelemetry Collector all read JSON lines from a
container's output or a file.

This Grafana Alloy pipeline sends a node's lines to Loki. It parses the fields
it needs, takes the time of the entry from `ts`, makes `level` a label and
keeps the logger name and the trace IDs as structured metadata:

```alloy
loki.process "fsc" {
  stage.json {
    expressions = {level = "", ts = "", name = "", trace_id = "", span_id = ""}
  }

  stage.timestamp {
    source = "ts"
    format = "Unix"
  }

  stage.labels {
    values = {level = ""}
  }

  stage.structured_metadata {
    values = {name = "", trace_id = "", span_id = ""}
  }

  forward_to = [loki.write.default.receiver]
}
```

It was checked with Alloy v1.22.0, which parses `ts` with its fractions as
`Unix`. Connect it to a source, such as `loki.source.file` or
`loki.source.docker`, and to a `loki.write` component.

Keep labels to values with few distinct values, such as `level`, the node and
the environment. A label for each transaction ID, trace ID or logger name makes
a stream per value and slows Loki down. Find such values with a line filter
(`|= "txid"`), with `| json`, or through structured metadata.

To link a line in Grafana to its trace, see
[Trace IDs in log lines](#trace-ids-in-log-lines).

## Trace IDs in log lines

When a `*Context` method such as `InfofContext` writes a line and its context
carries a span, the line has the fields `trace_id` and `span_id`. These are the
names the OpenTelemetry specification uses for log formats other than OTLP:

```text
2026-11-23 10:15:04.123 UTC [fsc.platform.view.services.view] RunView -> INFO 01a view started trace_id=4bf92f3577b34da6a3ce929d0e0e4736 span_id=00f067aa0ba902b7
```

A trace ID names one request, a view and the sessions it opens, on every node it
reaches over the websocket comm driver: to collect the lines that led to an
error, search all nodes for the error line's `trace_id`. A node with
`fsc.tracing.provider` set to `none` writes the IDs too, but exports no spans.
Its lines carry the span ID of the caller's span when a request comes from
another node, and the ID of a span it started itself, which no backend has,
otherwise. A trace that passes through such a node stays connected in Tempo or
Jaeger.

Lines without a context, and the lines of gRPC and of the libraries that log
through `flogging`, have no IDs. The libp2p comm driver doesn't send the span
to the other node: there, a responder's lines have a trace ID of their own.

In Grafana, a derived field on the Loki data source turns the trace ID into a
link to the trace. Its regular expression is `trace_id=(\w+)` for a pattern
format, and `"trace_id":"(\w+)"` with `logging.format: json`.

## Logging through the application's handler

A node writes its log lines in `logging.format` to standard error, or to the
`Writer` of `logging.Config`. An application that embeds FSC and has its own
logging setup can send FSC's lines to its own `slog.Handler` instead. The
standard library has text and JSON handlers, zap and OpenTelemetry ship one
(`zapslog`, `otelslog`), and adapters exist for most other logging libraries:

```go
logging.SetHandler(appHandler) // before building the node
```

Every FSC logger then writes through `appHandler`, including package-level
loggers created before the call. `logging.format` and `Writer` no longer apply;
`SetHandler(nil)` restores the default handler.

Two filters decide whether a line is written, in this order:

1. `logging.spec`, for the logger's name, which `PUT /logspec` changes while
   the node runs.
2. The handler's `Enabled` method.

To let the handler alone decide, set `logging.spec: debug`. At startup the node
logs, at Info, the spec and the type of the handler, which shows which filter
dropped a missing line.

The handler gets the logger name as the attribute `logger`; the fields of
`With`, of the `*w` methods and of `logging.RegisterContextLogField` as
attributes; the `trace_id` and `span_id` of a `*Context` line (see
[Trace IDs in log lines](#trace-ids-in-log-lines)) as attributes; and the caller
of the log method as the record's PC. A handler that reads the span from the
context itself, such as `otelslog`, writes these IDs twice. The `*Context`
methods also record the message as an event on the span in the context,
whatever the handler.

The handler also gets the lines of gRPC and of the libraries that log through
Fabric's `flogging`, such as the MSP, BCCSP and policy packages, under the same
two filters. These lines have no context fields and no stack trace, and they
lack the fields a library adds to its logger with `With`. zap's levels above
Error arrive as `ERROR+4` (DPanic), `ERROR+8` (Panic) and `ERROR+12` (Fatal).

With the libp2p comm driver, the handler also gets libp2p's lines; see
[libp2p log lines](#libp2p-log-lines).

The application can route its own `slog` lines through `logging.spec` and the
same handler, under a logger name of its choice:

```go
slog.SetDefault(slog.New(logging.Handler("app")))
```

FSC never calls `slog.SetDefault` itself. The handler passed to `SetHandler`
must not be `slog.Default().Handler()` after such a call, or any other handler
that writes through `logging.Handler`: each line would come back to it without
end.

## libp2p log lines

With the libp2p comm driver, libp2p's own lines go through FSC logging under the
logger name `libp2p`. They are written in `logging.format`, reach the handler
set with `SetHandler`, and follow `logging.spec` and `PUT /logspec`. The libp2p
subsystem, such as `basichost` or `swarm2`, is the attribute `subsystem`; there
is no level per subsystem.

`logging.spec` applies to `libp2p` as written, as to any other logger name: a
spec of `debug` includes libp2p's Debug lines. libp2p logs routine events at
Info and a great deal at Debug, so the specs FSC ships add `libp2p=warn`, which
keeps its warnings and errors, as they add `grpc=error`. A node with a spec of
its own adds the term itself:

```yaml
logging:
  spec: grpc=error:libp2p=warn:info
```

go-libp2p's `GOLOG_LOG_LEVEL`, `GOLOG_LOG_FORMAT` and `GOLOG_LOG_ADD_SOURCE`
environment variables don't apply to these lines; `GOLOG_LOG_LABELS` still adds
its labels as attributes. FSC doesn't need `LIBP2P_TCP_REUSEPORT`; setting it to
false keeps the subsystem `tcp-demultiplex` on go-libp2p's own stderr handler.

## What is never logged

FSC's rule is to never log raw state values, payloads, private keys or
credentials, at any level, in log lines or in error messages, and to log a
hash, a length or an identifier instead. The rule for developers is in
[Conventions → Logging & monitoring](../../../dev/conventions.md#logging--monitoring).
