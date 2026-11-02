# View Runtime Security Model

## Overview

The Fabric Smart Client (FSC) view runtime operates under a **shared process security model** where all registered views execute within the same process space without isolation. This document describes the current security model, its implications, and best practices for secure deployment.

## Security Model

### Trusted Computing Base (TCB)

**All view code registered at an FSC node is considered part of the Trusted Computing Base (TCB).** This means:

- View code has full access to the process memory space
- Views share the same execution context without isolation
- Malicious or compromised view code can affect the entire system
- **Once a view is loaded, it has complete access to all system resources**

### Lack of View Isolation

Unlike containerized or sandboxed execution environments, FSC views currently execute without isolation boundaries:

- **Shared Context**: Multiple views share the same `view.Context` object
- **Service Override Risk**: Malicious views can override critical services (signer, network services, etc.)
- **Cross-View Contamination**: Modifications made by one view persist and affect all other views
- **No Resource Limits**: Views are not restricted in CPU, memory, or I/O usage

### Dependency Chain Trust

The security model extends beyond your own view code:

> **"Once your dependencies are in, they are in. While I may trust your code, I may not trust your dependencies."**

- All transitive dependencies become part of the TCB
- Compromised dependencies can exploit the same vulnerabilities as malicious view code
- Supply chain attacks are a significant concern

## Data Received from Peers

A Fabric transaction a node receives from a remote peer, before it is signed, endorsed, or stored,
passes through `endorser.ValidateReceived` (`platform/fabric/services/endorser/validate.go`). This
closes a gap the shared-TCB model above does not cover on its own: the remote peer is untrusted
input, not loaded view code, and could otherwise choose the txID its transaction is stored under
or present a creator that disagrees with its proposal.

`ValidateReceived` checks that:

- the txID equals `ComputeTxID(nonce, creator)` (Fabric networks only; see below for Fabric-x);
- a signed proposal, if the transaction carries one, is an `ENDORSER_TRANSACTION` with epoch `0`,
  a non-empty nonce and creator, and a header whose txID, nonce, and creator all equal those of the
  transaction;
- the creator is valid for the channel's MSPs.

It does **not** verify the proposal signature against the creator. FSC builds a proposal with the
transaction's creator in the header, but the first identity that endorses signs it, which need not
be the creator — anonymous-creator and HSM-based endorser flows rely on this. A node that wants
signer-equals-creator must check that itself.

On Fabric-x networks the txID-equality check is skipped: the Fabric-x committer does not require
`txid == ComputeTxID(nonce, creator)` for data transactions, and a Fabric-x proposal response is
bound to its transaction only by the txID, not by a proposal hash. FSC skips the check there
because the committer does not enforce it, so on Fabric-x a remote peer can still choose the txID
its transaction is stored under. The proposal header consistency checks and the creator check still
apply on Fabric-x.

Validation runs on the two paths that take a transaction from a remote peer: decoding it
(`endorser.NewTransactionFromBytes`, used by `state.ReceiveTransaction`) and, again, right before an
`endorseView` signs it. It does not run when the committer reloads a node's own previously stored
transactions, so a creator certificate that expired or was revoked after endorsement does not block
that transaction from committing. The lower-level decoding constructors
(`fabric.TransactionManager.NewTransactionFromBytes`, `endorser.Builder.NewTransactionFromBytes`,
`Transaction.SetFromBytes`) do not validate either; call `ValidateReceived` explicitly if building
a transaction from bytes outside those two paths.

A certification envelope (`platform/fabric/services/state/certification.go`, decoded with
`endorser.NewTransactionFromEnvelopeBytes`) also comes from a peer, but it does not pass through
`ValidateReceived`: the node only decodes it to check its endorsements against the peers discovery
returns and to read the certified value, and never endorses or stores it.

A rejected transaction's error wraps `endorser.ErrInvalidTransaction` and is counted and logged as a
security rejection (see [Security metrics](services/monitoring_metrics.md#security)). While the
channel's MSP configuration is not loaded yet, such as at startup or during catch-up, the check
instead returns an error wrapping `driver.ErrNotInitialized`, which a retry can clear, and is not
counted as a rejection. If the node received a channel configuration and refused it, the check
returns an error wrapping `driver.ErrConfigRejected`, also not counted; a retry does not clear it,
so surface it to the operator instead of retrying.

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
  store's `PutBindings` and `ReplaceBinding` refuse such an ephemeral with an error wrapping
  `driver.ErrBindingRefused` and write no row for it; the other ephemerals of a `PutBindings` call
  are still bound.
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
  row; the operator check in the [upgrade guide](../../upgrade/v0.27-to-v0.28.md#binding-store)
  does.

The no-redirect rule protects only identities the node already knows. An identity with neither a
self-row nor a resolver, such as a party this node has not exchanged with yet, can be bound as
another party's ephemeral by an unproven `Bind`, and later unproven binds to it then resolve to
that party: the first write wins. Proven binds are the protection against this. `BindProven` is
not redirected by such a row, so the recipient exchange, which binds after a proof of ownership,
stores the correct owner.

`Bind` itself does not check ownership. Binding an identity received from a remote party is a
trust decision: call `Bind` only for identities from local configuration or for which the caller
holds a proof of ownership.

## Best Practices

### 1. Strict View Code Review

- **Audit all view code** before deployment
- Implement code review processes for all views
- Never deploy views from untrusted sources

### 2. Dependency Management

- **Audit all dependencies** and their transitive dependencies
- Use dependency scanning tools (e.g., `go mod graph`, vulnerability scanners)
- Pin dependency versions to prevent supply chain attacks
- Regularly update dependencies to patch known vulnerabilities
- Consider vendoring dependencies for reproducible builds

### 3. Access Control

- Use separate FSC nodes for different trust domains
- Never mix trusted and untrusted views on the same node
- Every client on `fsc.view.remote.clients` may call every view unless a policy checker narrows it;
  see [Remote View API Access Control](services/view-service.md#remote-view-api-access-control)

## Conclusion

The current FSC view runtime security model requires **complete trust in all deployed view code and dependencies**. Developers must implement strict governance and access controls to ensure only trusted code executes on FSC nodes. 

**Key Takeaway**: Treat view deployment with the same security rigor as deploying system-level code or kernel modules. Once malicious code enters the process space, the entire node is compromised.

---

*This security model documentation reflects the current state of FSC. Always check the latest releases and security advisories for updates.*
