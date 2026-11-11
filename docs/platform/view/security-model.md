# View runtime security model

## Overview

The Fabric Smart Client (FSC) view runtime operates under a **shared process security model** where all registered views execute within the same process space without isolation. Remote parties are a separate boundary: the organization is the unit of trust, and FSC nodes and peers of other organizations may be malicious. This page covers both boundaries: what view code can do inside the process, the write-once stores, identity bindings and P2P sessions that protect a node from remote parties, and how to deploy a node with that in mind. The checks the Fabric platform applies to transactions and identities received from peers are in [Fabric security](../fabric/security.md).

## Security model

### Trusted computing base (TCB)

**All view code registered at an FSC node is considered part of the Trusted Computing Base (TCB).** This means:

- View code has full access to the process memory space
- Views share the same execution context without isolation
- Malicious or compromised view code can affect the entire system
- **Once a view is loaded, it has complete access to all system resources**

### Lack of view isolation

Unlike containerized or sandboxed execution environments, FSC views execute without isolation boundaries:

- **Shared Context**: Multiple views share the same `view.Context` object
- **Service Override Risk**: Malicious views can override critical services (signer, network services, etc.)
- **Cross-View Contamination**: Modifications made by one view persist and affect all other views
- **No Resource Limits**: Views are not restricted in CPU, memory, or I/O usage

### Dependency chain trust

The security model extends beyond your own view code:

- All transitive dependencies become part of the TCB
- Compromised dependencies can exploit the same vulnerabilities as malicious view code
- Supply chain attacks are a significant concern

## Data received from peers

A Fabric node validates every transaction it receives from a remote peer before it signs,
endorses or stores it. See [Data received from peers](../fabric/security.md#data-received-from-peers).

## Recipient proof of ownership

The Fabric recipient views bind a recipient identity to the sender's node only after checking the
sender's proof that it controls the identity. See
[Recipient proof of ownership](../fabric/security.md#recipient-proof-of-ownership).

## Write-once stores

The envelope store, the endorsed-transaction store and the audit-info store hold records that
other components trust once written: the committer loads the read-write set of a transaction from
its stored envelope or endorsed transaction, and the audit info of an identity decides what an
auditor sees. A
remote peer that learns a txID or an identity could otherwise race the honest writer and have its
own record kept. These stores are therefore write-once:

- Writing the record already stored under a key is a no-op and returns nil, so retries and
  replicas keep working.
- Writing a different record returns an error wrapping `driver.ErrConflict`
  (`platform/view/services/storage/driver`) and keeps the stored record.

The SQL stores insert with `ON CONFLICT DO NOTHING` and, when no row was inserted, read the stored
record back and compare bytes. The envelope and endorsed-transaction stores refuse an empty
record, which their existence check would report as absent. Their rows are never deleted, and
updated only by the endorse-tx store's contract step 2, which rewrites a JSON transaction as the
envelope of the same transaction (see [Schema migrations](services/schema-migrations.md#contract-steps)).
The record read back is therefore the one that won, and of two concurrent writers of different
records exactly one succeeds.

How the Fabric stores compare records, how a conflict is reported, and how the Fabric committer
handles an envelope forged with an honest transaction's txID are described in
[Fabric security](../fabric/security.md#write-once-stores).

## Identity bindings

The endpoint service resolves an identity a node does not know directly, such as a recipient
identity a remote party sent, through the binding store: a table that maps each such ephemeral
identity to the long-term identity of the party that owns it. A long-term identity is bound to
itself (its self-row). Which party an identity resolves to decides where messages for it are sent
and whose signature counts for it, so the store and `endpoint.Service`
(`platform/view/services/endpoint/service.go`) apply these rules:

- **No redirect.** A known long-term identity is never stored as the ephemeral of another
  identity. A known long-term identity is one that has a self-row, or that is the identity of a
  registered resolver; a resolver's name, aliases and addresses do not count. Binding one would
  redirect every later lookup of that identity, and every later bind to it, to another party. The
  store's `PutBindings` and `ReplaceBinding` refuse an ephemeral with a self-row with an error
  wrapping `driver.ErrBindingRefused` and write no row for it; the other ephemerals of a
  `PutBindings` call are still bound. The endpoint service refuses a resolver's identity before it
  reaches the store: `Bind` skips it, and `BindProven` returns an error wrapping
  `driver.ErrBindingRefused`.
- **Unproven binds do not take over.** `Service.Bind` keeps the existing row for an ephemeral that
  is already bound to a different long-term identity. Both a conflict and a refusal are counted
  once per ephemeral, as `binding_conflict` and `binding_refused` (see
  [Security metrics](services/monitoring_metrics.md#security)), and logged at warn level, at most
  one line per kind per second. `Bind` still returns nil, so
  configuration-driven binds at startup and application binds keep working.
- **Proven binds win.** `Service.BindProven` is for callers that verified that the long-term
  identity proved ownership of the ephemeral. It binds the ephemeral to exactly that long-term
  identity, never to an identity the long term is itself bound to, replaces an existing binding of
  the ephemeral, logs the replacement at warn level, and returns the `ErrBindingRefused` error for
  a known long-term identity. A proven bind therefore replaces a row another party planted for
  the proven ephemeral. It does not repair a planted row on the long-term identity itself: the
  long term's self-row is written only if the long term has no row yet. Binds never remove such a
  row; an operator does, as described below.

A planted row is one whose `ephemeral_hash` is the hash (`base64(sha256(identity bytes))`) of an
identity that is itself the `long_term_id` of some row, while the row binds it to a different
long-term identity. In the default store the table is `fsc_default_bind`. Many such rows are
legitimate: a node binds its own identities to each other (its Fabric MSP identity to its FSC
identity, and an `endpoint.resolvers` entry to the FSC identity of the resolver with the same
name), and endorsement checks rely on them. To find planted rows, list these candidates, decode
both identities and compare the certificates they carry. Delete a row only if its two identities
clearly belong to different parties; an identity without a certificate, such as an Idemix
identity, cannot be attributed this way, so keep its rows. The
[v0.27 to v0.28 upgrade guide](../../upgrade/v0.27-to-v0.28.md#binding-store) has the queries
for Postgres and SQLite.

The no-redirect rule protects only identities the node already knows. An identity with neither a
self-row nor a resolver, such as a party this node has not exchanged with yet, can be bound as
another party's ephemeral by an unproven `Bind`, and later unproven binds to it then resolve to
that party: the first write wins. Proven binds are the protection against this. `BindProven` is
not redirected by such a row, so the recipient exchange, which binds after a proof of ownership,
stores the correct owner.

`Bind` itself does not check ownership. Binding an identity received from a remote party is a
trust decision: call `Bind` only for identities from local configuration or for which the caller
holds a proof of ownership.

## P2P sessions

The transport authenticates the FSC node that sends a message, but any authenticated node can send
a first message for any responder registered on this node, and a responder reads input that node
chose. A node limits what such a peer can start:

- **Who may start a responder**: a responder registered with `view.WithAuthorizer` runs only for
  the nodes its authorizer accepts; a refusal is counted as the security rejection `unauthorized`.
  See [Authorizing responders](programming-model.md#authorizing-responders).
- **How many responders run**: `fsc.p2p.maxRespondersPerPeer` bounds the responders one peer runs
  at once, and `fsc.p2p.maxResponders` those of all peers together, so that many peers within their
  own limit cannot exhaust the node. See [Error replies a node sends unasked](services/comm/readme.md#error-replies-a-node-sends-unasked).
- **What the initiator learns**: a first message the node refuses gets at most an error reply
  with a fixed reason, never the authorizer's error text; some are dropped without a reply. The
  initiator can test the reason with `errors.Is` against the `session` sentinels, but the reason
  is what the remote node claims, not proof that a check ran.

What a responder does with an accepted message is the responder's responsibility: it must validate
every payload, as [Responder responsibilities](programming-model.md#responder-responsibilities)
describes.

## Best practices

### 1. Strict view code review

- **Audit all view code** before deployment
- Implement code review processes for all views
- Never deploy views from untrusted sources

### 2. Dependency management

- **Audit all dependencies** and their transitive dependencies
- Use dependency scanning tools (e.g., `go mod graph`, vulnerability scanners)
- Pin dependency versions to prevent supply chain attacks
- Regularly update dependencies to patch known vulnerabilities
- Consider vendoring dependencies for reproducible builds

### 3. Access control

- Use separate FSC nodes for different trust domains
- Never mix trusted and untrusted views on the same node
- Every client on `fsc.view.remote.clients` may call every view unless a policy checker narrows it;
  see [Remote view API access control](services/view-service.md#remote-view-api-access-control)
