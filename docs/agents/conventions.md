# Conventions

## Code organization

- **Platform-specific code** → `platform/<platform-name>/`
- **Integration tests** → `integration/<platform-name>/<test-name>/`
- **Shared utilities** → `pkg/utils/` or `platform/common/`
- **Network orchestration** → `integration/nwo/`

## Error handling

Use `github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors`. Do **not**
build or wrap errors with `fmt.Errorf` (the codebase uses `errors.Wrapf`/`Errorf`
~10x more than `fmt.Errorf`, and downstream projects such as Panurus enforce this).

Permitted constructors: `errors.New`, `errors.Errorf`, `errors.Wrap`,
`errors.Wrapf`, `errors.WithMessage`, `errors.WithMessagef`, `errors.Join`.

- Wrap with context: `errors.Wrapf(err, "context: %s", detail)`
- Combine multiple errors: `errors.Join(...)`
- Handle errors explicitly; don't discard them with the blank identifier.

## Godoc

Give every exported function, type, and package a Godoc comment. Keep
documentation short, precise, and behavior-focused.

- Start the comment with the identifier's name and describe what it does or
  represents **from the caller's perspective**.
- Document only what an API user needs, especially non-obvious behavior: error
  semantics, blocking or context behavior, concurrency guarantees,
  ownership/lifecycle requirements, defaults, side effects, security properties.
- Do **not** describe implementation details, internal call flows, or behavior
  that cannot be verified from the code, tests, or API contract. Never invent
  guarantees.
- Use complete sentences, consistent FSC terminology, and Godoc links such as
  `[Config]` when referring to other API elements.
- Prefer one accurate sentence over several speculative or redundant ones, and
  keep documentation stable under implementation refactoring.

## Logging & monitoring

- Structured logging via `platform/common/services/logging`.
- OpenTelemetry for traces and metrics; OTLP setup lives in
  `platform/view/services/tracing`.
- Jaeger (tracing) and Prometheus (metrics) are the typical backends.

## Storage drivers

Registered into the `db-drivers` DI group (view stores) and the
`fabric-db-drivers` group (Fabric stores):

- **SQLite** (`modernc.org/sqlite`) — development.
- **PostgreSQL** (`github.com/jackc/pgx/v5`) — production.
- **Memory** — tests; also the fallback for a store that names no persistence
  when `fsc.persistences.default` is not defined.

Keys are compared bytewise and may contain U+0000 (composite keys). Bind them so
the column stores them verbatim — `BYTEA` on Postgres and for the SQLite KVS,
`TEXT` for the SQLite vault — rather than escaping them into another string: an
escape scheme must stay injective and order-preserving, and range scans silently
break when it is not. A range query needs `ORDER BY pkey`: without it SQL returns
rows in whatever order the plan produces.

## Identity management

- An identity is a container for lower-level identity types: X509, ECDSA, Idemix.
- FSC nodes can act as Fabric endorsers when given the proper key material.
- Identity providers are registered via DI.
- HSM key material is supported through PKCS#11 (`github.com/miekg/pkcs11`).

See [`docs/platform/view/security-model.md`](../platform/view/security-model.md)
for the trust and identity model.

## Configuration

- One `core.yaml` per node.
- `fsc` section → view platform; `fabric` section → Fabric.
- Detailed examples: [`docs/configuration.md`](../configuration.md).
- `docs/configuration.md` is pinned by reference tests built on
  [`pkg/utils/configtest`](../../pkg/utils/configtest): each platform's
  `TestReferenceConfiguration` (`platform/view/services/config`,
  `platform/fabric/core/generic/config`, `platform/fabricx/core/committer/config`) claims
  its sections, strictly decoding the ones read into a struct and listing the keys read one
  at a time; `TestReferenceResolvers` (`platform/view/sdk/dig/support/endpoint`) checks
  `fsc.endpoint.resolvers`. A documented key that no test claims fails, and
  `TestReferenceSectionsOwned` (`pkg/utils/configtest`) fails on a top-level section with no
  owning test. Adding, renaming or removing a key means updating the document and the
  reference test of the component that reads it.

## Security

- TLS is configurable for all network communication.
- View sessions verify peer identity; endorsement policies are validated.
- Keep endorser/HSM key material out of application views.
