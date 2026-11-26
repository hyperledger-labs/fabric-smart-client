# A transparency log on Chaguar

This example runs a Rekor-style transparency log on Chaguar: three
organizations, one log node each, append signed artifact records to a shared
Merkle tree and sign checkpoints over it together. It is modeled on
[Rekor](https://github.com/sigstore/rekor), the transparency log of
[Sigstore](https://www.sigstore.dev/), and reuses several of its packages, but
it is not Rekor and does not speak Rekor's API; this demo is called `tlog`.

Rekor itself is one service: a client hashes and signs an artifact, submits it
to Rekor, and Rekor appends the entry to its Merkle tree and signs checkpoints
(tree size and root hash) with its own key. Every client trusts that one
operator's signature. If that operator is compromised, careless, or decides
to misbehave, nothing stops it from rewriting its history or showing two
different clients inconsistent views of the log: it is the only party that
ever signs a checkpoint.

This demo turns that one operator into three. Every entry needs all three
organizations' endorsement before it commits, the order transactions commit
in on the ledger fixes each entry's position in the tree, and a checkpoint
counts only once all three have signed it. A client that talks to any one
node, honest or not, can still catch a lie, because it never trusts an
answer, only the proofs and signatures behind it.

## The parties

| FSC node | Organization | Role                                                                                       |
| -------- | ------------ | ------------------------------------------------------------------------------------------ |
| `org1`   | `Org1`       | runs a log node: endorses entries, cosigns checkpoints, serves `log`/`get`/`checkpoint`    |
| `org2`   | `Org2`       | the same as `org1`                                                                         |
| `org3`   | `Org3`       | the same, plus the demo's cheat switch (see [The misbehaving node](#the-misbehaving-node)) |

[`topology.go`](topology.go) defines the network, one namespace, `tlog`, with
an endorsement policy that requires all three organizations
(`topology.Unanimity("Org1", "Org2", "Org3")`), and registers every node's
views. The ledger holds two kinds of key in that namespace: `size`, the
number of entries, and `entry_<i>`, the canonicalized body of the entry at
index `i`, which is the leaf the tree hashes.

## Logging an entry

A client signs the SHA-256 digest of an artifact with its own ECDSA P-256
key, builds a `HashedRekordRequestV002`, and calls the `log` view on any one
node, its entry node. [`views/log.go`](views/log.go):

1. `translog.CanonicalEntry` verifies the client's signature and turns the
   request into Rekor's canonicalized JSON entry body, the same bytes Rekor
   itself would hash into the tree.
2. The entry node builds a transaction in the `tlog` namespace that reads
   `size = n` and writes `entry_n` and `size = n+1`. Reading `size` is what
   makes a second, concurrent log transaction conflict with this one: without
   it, both writes would be blind, and whichever committed later would
   silently overwrite the first entry.
3. It collects endorsements from the other two nodes
   (`state.NewCollectEndorsementsView`). Each endorser's `LogResponderView`
   checks that the proposer passes the channel ACL, then rebuilds the
   canonical entry itself from the transient request and checks that the
   transaction reads `size` at the version the endorser reads on its own
   ledger, that its only writes are exactly `entry_n` and `size = n+1` for the
   `n` the endorser read there, and that the written entry is byte-for-byte
   what it just rebuilt. Without the version check, a proposer that reads
   `size` at a newer version than a lagging endorser could get it to approve
   rewriting an entry that already exists, and the committer would accept the
   transaction, since its read is current.
4. It orders the transaction and waits for finality. A transaction another,
   concurrent `log` beat to the same index comes back invalid; the entry node
   retries with fresh endorsement sessions, up to three times, before giving
   up.
5. Once committed, it asks for a checkpoint covering the new entry (see
   [Checkpoints and cosigning](#checkpoints-and-cosigning)) and returns the
   index, the canonical body, an inclusion proof against that checkpoint, and
   the checkpoint itself, as a `TransparencyLogEntry`.

[`client/client.go`](client/client.go)'s `Client.Log` does the client side of
all this, then checks what comes back: it rebuilds the canonical entry from
its own signature and compares it to `CanonicalizedBody`, checks that the
checkpoint in the inclusion proof carries all three orgs' signatures, and
checks the inclusion proof itself. Only then does it call `Audit` to catch
up its own view of the log.

## Checkpoints and cosigning

Any node asked for a checkpoint, whether through `log`, `get`, or
`checkpoint`, builds one the same way (`LogService.checkpointAt` in
[`views/service.go`](views/service.go)): it syncs its tree from the ledger
(reads `size`, fetches the missing `entry_i` keys with the Chaguar query
service's `GetStates`, and appends them), signs `(size, root)` with its own
log key, and asks its two peers over an FSC session
(`CosignView`/`CosignResponderView` in [`views/cosign.go`](views/cosign.go))
to cosign the same pair. A peer rebuilds its own tree from its own ledger
reads rather than use the asker's, waits a bit for its tree to catch up if
it is behind, and only then compares roots. It signs if they match, and
otherwise refuses with a reason: `mismatch` if its root at that
size differs, `behind` if it still hasn't reached that size by the time it
gives up waiting. The signatures merge into one note
(`translog.MergeSignatures`).

Nothing is cached or pushed ahead of time: every checkpoint request is two
fresh round trips to the other two orgs. "Latest" just means the latest tree
the node serving the request has itself read off the ledger, so two clients
going through different nodes can briefly see different sizes; what matters
is that the consistency proof between their checkpoints holds, which it
always does for two checkpoints an honest node computed from the same
ledger.

## Reading an entry and auditing the log

`GetView` ([`views/read.go`](views/read.go)) syncs the serving node's tree,
rejects an index at or past its current size, and otherwise returns the entry
the same way `log` does: canonical body, inclusion proof, three-signed
checkpoint. `Client.Get` checks more than the proof: it checks that the
entry's own `LogIndex` is the index that was asked for, and that the
inclusion proof is for that same index, before verifying the proof itself.
Without that check, a node could answer any `Get` with some other entry it
can honestly prove, just not the one asked for.

`CheckpointView` syncs and cosigns the current head, reports any refusals
instead of failing on them (unlike the plain `Checkpoint` helper that `log`
and `get` use), and adds a consistency proof against an older size on
request. `Client.Audit` calls it with the size of the checkpoint it already
holds, verifies the new checkpoint carries all three signatures, and verifies
that it is consistent with the one it holds, before replacing it. The
client's `Latest` field only ever advances once both checks pass; a
checkpoint that fails either check leaves `Latest` where it was.

## What comes from rekor-tiles and transparency-dev, and what is FSC's

| Need | Source |
| --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Request type | rekor-tiles `pkg/generated/protobuf`: `HashedRekordRequestV002` in |
| Entry type | `sigstore/protobuf-specs`: `TransparencyLogEntry` out |
| Signature check and canonical entry | rekor-tiles `pkg/types/hashedrekord.ToLogEntry`, plus JSON canonicalization the same way Rekor does it |
| Checkpoint text, with this demo's own origin | `transparency-dev/formats/log.Checkpoint`, under the origin `tlog.example` |
| Checkpoint signing and verification | rekor-tiles `pkg/note` (`NewNoteSigner`/`NewNoteVerifier`) over a `sigstore/sigstore` ECDSA signer/verifier |
| Inclusion and consistency proof verification | rekor-tiles `pkg/verify`: `VerifyInclusionProof`, `VerifyWitnessedCheckpoint`, `VerifyConsistencyProofWithCheckpoints` |
| The Merkle tree itself, and its proofs | `golang.org/x/mod/sumdb/tlog` (`StoredHashes`, `TreeHash`, `ProveRecord`, `ProveTree`), the same RFC 6962 hashing rekor-tiles' verifiers expect |

What the demo writes itself, in [`translog/`](translog) and
[`views/`](views): the FSC views that log, endorse, cosign, read and
checkpoint; growing the tree from ledger order instead of from a log
process's own memory; the 3-of-3 rule on top of `VerifyWitnessedCheckpoint`,
which by itself only requires the first verifier's signature
(`translog.VerifyAll`); and the misbehaving node below.

## The misbehaving node

Org3's node registers one extra, test-only view, `cheat`
([`views/cheat.go`](views/cheat.go)), that switches its `get` and
`checkpoint` views between three modes. Org1 and org2 never register
`cheat.go`'s views at all; their `get` and `checkpoint` are the plain,
honest ones.

- `rewrite-entry`: `get` flips one byte of the entry body it returns, keeping
  the real inclusion proof. The proof was computed over the real bytes, so it
  no longer matches the tampered ones: `verify.VerifyInclusionProof` fails,
  and the client sees `failed verifying the inclusion proof`. A client
  talking to org1 or org2 for the same index gets the honest entry, proof
  intact.
- `fork`: `checkpoint` drops the entry at index 1 and builds a second,
  divergent tree over what is left (dropping the last entry instead would
  just give the honest, smaller tree, which org1 and org2 would rightly
  cosign). Org3 signs that tree's root and asks org1 and org2 to cosign it;
  both rebuild their own, real trees and refuse with `mismatch`, so the
  merged note carries only org3's signature. A client auditing against it
  gets `missing signature`, not a wrong answer it might act on. Once org3 is
  honest again, its next checkpoint is, correctly, consistent with the one
  the client already held from before the fork.

Both scenarios are specs in [`tlog_test.go`](tlog_test.go): "catches a node
that rewrites an entry" and "catches a node that forks the log".

Under the policy this demo uses, every entry needs all three organizations'
endorsement and every checkpoint needs all three signatures, so a
misbehaving organization also has a quieter option: just refuse. Refuse to
endorse, and nothing new gets logged; refuse to cosign, and no node can
produce a checkpoint anyone will accept, so nothing new gets read either. It
can stall the log that way, but it cannot make a client accept a lie.

## Log keys

Each organization signs checkpoints with its own ECDSA P-256 key, separate
from the FSC node's own identity key. A node learns its key, and the other
two orgs' public keys, through the `init` view
([`views/setup.go`](views/setup.go)): `Setup` carries the node's private key
(PKCS8 DER), all three orgs' public keys (PKIX DER, in `Org1`..`Org3` order),
and the node's two peers, and `InitView` parses them and hands them to the
node's `LogService`, in memory only. [`tlog_test.go`](tlog_test.go) generates
the three keypairs and calls `init` on every node; a client only ever needs
the three public keys to verify what any node hands it.

Nothing here loads a key from `core.yaml`, a vault, or an HSM: that part is
left out on purpose. A real deployment would load each org's log key the way
it already loads other node key material, and distribute the public keys out
of band, rather than handing a private key to a node over a view call the
way this demo's test does.

## Running the example

The suite needs Docker and the Chaguar tools; see
[Integration tests](../../docs/dev/integration-tests/README.md) for the
setup. Pass an absolute `FAB_BINS`:

```bash
make integration-tests-examples-tlog FAB_BINS=/absolute/path/to/fabric/bin
```

[`tlog_test.go`](tlog_test.go) runs the whole story on one network: entries
logged through different nodes, a rejected bad signature, concurrent logs
racing for the same index, every node serving the same log and proving the
same old entry, an audit that follows the log as it grows, and the two cheat
scenarios above.
