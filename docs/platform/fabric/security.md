# Fabric security

A Fabric node runs under the
[View runtime security model](../view/security-model.md): view code is trusted,
and FSC nodes and peers of other organizations may be malicious. This page
describes what the Fabric platform checks in transactions, recipient identities
and stored records that come from such peers.

## Data received from peers

A Fabric transaction a node receives from a remote peer, before it is signed,
endorsed, or stored, passes through `endorser.ValidateReceived`
(`platform/fabric/services/endorser/validate.go`). The remote peer is untrusted
input, not loaded view code, and could otherwise choose the txID its transaction
is stored under or present a creator that disagrees with its proposal.

`ValidateReceived` checks that:

- the nonce and the creator are not empty;
- the txID equals `ComputeTxID(nonce, creator)` (Fabric networks only; see below
  for Fabric-x);
- a signed proposal, if the transaction carries one, is an
  `ENDORSER_TRANSACTION` with epoch `0`, a non-empty nonce and creator, and a
  header whose txID, nonce, and creator all equal those of the transaction;
- the creator is valid for the channel's MSPs.

It does **not** verify the proposal signature against the creator. FSC builds a
proposal with the transaction's creator in the header, but the first identity
that endorses signs it, which need not be the creator; anonymous-creator and
HSM-based endorser flows rely on this. A node that wants signer-equals-creator
must check that itself.

On Fabric-x networks the txID-equality check is skipped: the Fabric-x committer
does not require `txid == ComputeTxID(nonce, creator)` for data transactions,
and a Fabric-x proposal response is bound to its transaction only by the txID,
not by a proposal hash. FSC skips the check there because the committer does not
enforce it (the Fabric-x transaction format sets
`protos.TransactionFormat.SkipTxIDCheck`; a driver without a registered format
is checked), so on Fabric-x a remote peer can still choose the txID its
transaction is stored under. The proposal header consistency checks and the
creator check still apply on Fabric-x.

Validation runs on the two paths that take a transaction from a remote peer:
decoding it (`endorser.NewTransactionFromBytes`, used by
`state.ReceiveTransaction`, and `endorser.NewTransactionFromPeer`, used by
`state.ReceiveTransactionFrom` for bytes from a known peer) and, again, right
before an `endorseView` signs it. It does not run when the committer reloads a
node's own previously stored transactions, so a creator certificate that expired
or was revoked after endorsement does not block that transaction from
committing. The lower-level decoding constructors
(`fabric.TransactionManager.NewTransactionFromBytes`,
`endorser.Builder.NewTransactionFromBytes`, `Transaction.SetFromBytes`) do not
validate either; call `ValidateReceived` explicitly if building a transaction
from bytes outside those two paths.

Signatures are verified over the bytes the signer produced, which either form of
a transaction keeps byte for byte (see
[Messages between nodes](fabric-sdk.md#messages-between-nodes)): an endorsement
over the `payload` bytes of the received `ProposalResponse` followed by the
endorser's identity, and a proposal signature over the `proposal_bytes` of the
received `SignedProposal`. Those fields and the signatures (`signature` of the
`SignedProposal`, the `endorsement` of each `ProposalResponse`) are copied as
they are; the outer `SignedProposal` and `ProposalResponse` messages are decoded
and marshaled again when a transaction is converted between its forms. No
signature covers the rest of the outer `ProposalResponse` (its `response` status
and message, `version` and `timestamp`), the routing fields of a
`TransactionEnvelope` (`platform`, the channel key, `network_hint`), or the
transaction fields outside the signed proposal, such as the transient map. A
receiver treats them as unauthenticated: it checks the envelope's channel
against the channel header of the signed proposal, and the routing key only
selects among the node's own networks.

A certification envelope (`platform/fabric/services/state/certification.go`,
decoded with `endorser.NewTransactionFromEnvelopeBytesOn`) also comes from a
peer, but it does not pass through `ValidateReceived`: the node only decodes it
to check its endorsements against the peers discovery returns and to read the
certified value, and never endorses or stores it.

A rejected transaction's error wraps `endorser.ErrInvalidTransaction` and is
counted and logged as a security rejection (see
[Security metrics](../view/services/monitoring_metrics.md#security)). While the
channel's MSP configuration is not loaded yet, such as at startup or during
catch-up, the check instead returns an error wrapping
`driver.ErrNotInitialized`, which a retry can clear, and is not counted as a
rejection. If the node received a channel configuration and refused it, the
check returns an error wrapping `driver.ErrConfigRejected`, also not counted; a
retry does not clear it, so surface it to the operator instead of retrying.

## Recipient proof of ownership

The recipient views in `platform/fabric/services/state/recipients.go`
(`RequestRecipientIdentityView`, `ExchangeRecipientIdentitiesView` and their
responders) send a recipient identity X together with a proof that the sender
controls it. The receiver binds X to the sender's node only after checking that
proof. Without a proof, a peer could claim another party's recipient identity.
It would then receive that party's traffic and could pass off its own
endorsement as that party's: the endorsement check accepts an endorser bound to
the party it asked.

The proof, carried in `RecipientData.Proof`, is X's signature over:

```text
0x00 || tag || sessionID || contextID || signerNode || verifierNode || X
```

Every field is length-prefixed. The fields and the reason for each:

- The leading `0x00` makes the payload unparsable as a protobuf message, so the
  signature cannot be confused with one over a proposal or envelope made with
  the same key.
- `tag` names the view and the direction: `fsc/recipient/v1/request-reply`,
  `fsc/recipient/v1/exchange-request` or `fsc/recipient/v1/exchange-reply`. A
  proof for one message is invalid for any other.
- `sessionID` and `contextID` tie the proof to one session. No extra round trip
  is needed.
- `signerNode` and `verifierNode` are FSC node identities: the identity from
  `fsc.identity` that the resolver maps to the peer's transport key, not a
  Fabric identity. The receiver fills them in from its own view of the session.
  `verifierNode` is its own node identity. `signerNode` is
  `Session().Info().Caller` on a responder, and on an initiator the resolver
  identity of the session's `RemotePKID`, or of the party dialed. A proof
  obtained from a victim names the relaying node as verifier and is refused by
  everyone else. Both node identities are encoded in the canonical PEM form of
  `id.CanonicalIdentity`, so a node whose certificate file differs in formatting
  from its peers' resolver configuration still produces matching proofs.

The sender signs with the Fabric network's signature service
(`fabric.NetworkService.SignerService()`). The receiver picks the verifier for X
from the form of X, and for a non-x509 `SerializedIdentity` from X's MSP in a
channel of its own configuration: the channel set with `state.WithChannel` on
the exchange views, or the network's default channel otherwise, and always the
default channel for `RequestRecipientIdentity`. The `Channel` field of an
`ExchangeRecipientRequest` is ignored, because the party being verified would
otherwise choose the MSPs that verify it.

- An x509 identity, a `SerializedIdentity` whose `IdBytes` hold a PEM
  certificate, is verified with the signature service and never needs the
  channel. It deserializes to a verifier for its public key if that key is an
  ECDSA key; any other key gives `no verifier`. It is not checked against any
  MSP, so a verified proof shows that the sender holds the key for X, not that X
  belongs to an MSP. Anything that needs membership must check it separately.
  The same holds for bytes that are not a `SerializedIdentity`.
- Any other `SerializedIdentity` needs the channel, to tell whether X's MSP is
  idemix. If it is an idemix MSP of the channel, X must be valid on the channel,
  which checks its credential against the issuer the channel configures for that
  MSP, and its verifier comes from the channel's MSPs. The receiver therefore
  verifies idemix recipients from any issuer in the channel, whether or not it
  holds a credential of its own. The validity check is required: the channel's
  verifier alone accepts a pseudonym with no credential behind it.
- If X's MSP is not an idemix MSP of the channel, X is verified with the
  signature service, through the deserializers of the node's locally configured
  MSPs. A local idemix MSP accepts X only under its own MSP ID and only with a
  valid credential from its issuer, so a pseudonym cannot claim the MSP ID of
  another organization.
- The signature service does not cache a verifier it deserializes for a proof
  check, so identities a peer sends cannot grow the node's memory. Verifiers
  already registered or cached, such as those of the node's own identities, are
  reused.

If X needs the channel and this node cannot use it, X is refused with an error
that carries the cause: the channel is unknown, for example because the network
has no default channel and no `state.WithChannel` is set; its configuration is
not in force yet (wrapping `driver.ErrNotInitialized`, which a retry clears once
the configuration arrives); or the node refused the configuration it received
(wrapping `driver.ErrConfigRejected`). Such a refusal is a condition of this
node, so it is not counted as a rejection, and the exchange responder tells the
initiator only `recipient verification unavailable: channel not ready`. x509
recipients are not affected.

The exchange responder verifies the initiator's proof before it replies. On
failure it replies with an error and binds nothing, so the initiator gets
`received error from remote [recipient proof: ...]`. A failed proof returns an
error wrapping `state.ErrRecipientProof`, is counted as `recipient_proof` (see
[Security metrics](../view/services/monitoring_metrics.md#security)), and binds
nothing. A verified identity is bound with `endpoint.Service.BindProven` to the
counterparty's node identity.

An idemix signature over this payload can be verified by anyone who holds the
issuer's public material, not only by `verifierNode`. Whoever sees the proof can
therefore link the idemix pseudonym to `signerNode`, which gives up idemix's
unlinkability toward observers of the proof.

Applications with their own recipient exchange can produce and check the same
proof with `state.SignRecipientProof` and `state.VerifyRecipientProof`.
Identities that cannot sign, such as remote wallets or script and multisig
owners, need their own exchange and handling.

## Write-once stores

The envelope store and the endorsed-transaction store are write-once, as
described in [Write-once stores](../view/security-model.md#write-once-stores).
This section covers how the Fabric platform compares records and handles a
conflict.

An endorsed transaction holds this node's proposal responses, whose signatures
differ on every signing. `ets.StoreTransaction`
(`platform/fabric/core/generic/transaction/services.go`) therefore treats two
records as the same when they carry the same signed proposal and all their
proposal responses carry the same payload, which holds the proposal hash and the
results. A record can have more responses than the stored one, as when the node
endorses the same transaction with a second identity. It decodes both records
first, so a JSON record and an envelope of the same endorsement compare equal.

A conflict is counted as the security rejection `store_conflict` (see
[Security metrics](../view/services/monitoring_metrics.md#security)).
`endorseView` reports the session's remote peer and fails the endorsement. The
Fabric committer reports an empty peer: a valid transaction whose envelope
conflicts is discarded alone, with a validation message containing
`store conflict`, and the channel keeps committing.

The Fabric committer stores only the envelopes of transactions Fabric found
valid. Anyone who knows a txID can sign an envelope carrying it with their own
identity and have it ordered ahead of the honest one. Fabric flags the forgery
invalid, as `BAD_PROPOSAL_TXID` when the txID is not derived from its creator,
but records a txID for duplicate detection within a block only when its envelope
is a valid endorser transaction, so the honest envelope later in the same block
is still `VALID`, also when the forgery has another header type. The paths that
resolve a txID from a block apply one rule, `committer.ShadowedEnvelopes`: the
verdict is that of the txID's valid envelope if the block has one, and otherwise
that of its first envelope. The committer skips the shadowed invalid envelope,
so the transaction's vault status and finality event are those of the honest
envelope. Remote finality checks (`delivery.DeliverReceive`), transaction scans
(`Delivery.Scan` and `ScanFromBlock`), ledger lookups
(`Ledger.GetTransactionByID`), and the delivery-based listener manager, which
does not pass a shadowed envelope to its `EventInfoMapper`, apply the same rule.
A ledger lookup gets the first envelope carrying the txID from a peer's qscc;
when that envelope is invalid, it fetches the envelope's block from the same
peer and returns the valid endorser transaction with the same txID if the block
has one. Every lookup of an invalid transaction fetches its block, forged or
not, and returns an error, not the invalid verdict, when it cannot read the
block. Block scans (`ScanBlock` and `ScanBlockFrom`) hand over whole blocks
unchanged; a callback that resolves txIDs from them calls
`committer.ShadowedEnvelopes` itself, as the listener manager does. Any other
invalid envelope marks its transaction Invalid without the committer storing it
or reading its read-write set. A forgery ordered in an earlier block is in
Fabric's ledger index, so Fabric flags the honest envelope `DUPLICATE_TXID` and
the transaction is invalid on every peer.

The transaction-metadata store keeps the first value written under a key and
returns nil on later writes: transient data is stored again during multi-step
endorsement.
