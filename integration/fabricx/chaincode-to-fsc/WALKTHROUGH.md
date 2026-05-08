# Walkthrough — Understanding This PoC From the Ground Up

This is the long-form companion to [README.md](README.md). The README is
the tutorial a reviewer reads in 20 minutes; this file is the *textbook*
that teaches you, line by line, what every piece of this PoC is doing
and why. If you have never used FSC before, start here.

By the end of this document you should be able to:

1. Explain what Hyperledger Fabric-X changes about the classical Fabric
   programming model.
2. Read any FSC view file and predict its behaviour.
3. Trace a full transaction from CLI to commit through every component.
4. Add a new chaincode method to the PoC by analogy.
5. Diagnose the common failure modes (timeouts, validation errors,
   missing responders).

---

## Part I — Background

### 1. What is Hyperledger Fabric, in one paragraph

Hyperledger Fabric is a permissioned blockchain. It runs **chaincode**
(smart contracts in Go, Java, or Node.js) on **endorsing peers**. A
client sends a transaction proposal to the endorsers; each endorser
*simulates* the chaincode against its local copy of the world state,
producing a **read-set** (keys read) and a **write-set** (keys written
and their new values). The client gathers a quorum of endorsements,
sends the bundle to the **orderer** (Raft or BFT), the orderer cuts a
block, and **committers** validate each transaction against the policy
and apply the write-set to the world state. The world state is stored
in LevelDB or CouchDB on each peer.

Three properties of classical Fabric are load-bearing for understanding
what Fabric-X changes:

- **Chaincode is the programmable surface.** All business logic lives
  inside chaincode containers; clients just send proposals.
- **The peer is a monolith.** One process per peer holds endorsement,
  validation, commit, and gossip.
- **The orderer is a separate ordering service.** Throughput is bounded
  by the orderer's BFT performance, often around 1–3k TPS in practice.

### 2. What Fabric-X changes

Fabric-X is a re-architecture by IBM Research (Marcus Brandenburger and
team) plus the Fabric maintainers. It re-uses the Fabric identity
model, MSP, and endorsement-policy concepts but rebuilds the runtime.
The four big changes:

1. **Chaincode is gone.** There is no chaincode container, no
   chaincode lifecycle, no chaincode endorser. Instead, business logic
   runs inside **Fabric-Smart-Client (FSC) views** — Go objects that
   live in FSC node processes. A view constructs a transaction,
   gathers signatures peer-to-peer, and submits to the orderer.
2. **The peer is decomposed.** The Fabric-X-Committer is five
   microservices: Sidecar (block ingress), Coordinator (orchestration),
   Verification Service (signature checks), Validator-Committer
   (write-set application), and **Query Service** (read-side).
3. **Arma orderer.** A sharded BFT consensus optimised for high
   throughput. Routers accept transactions, batchers persist them,
   consenters run BFT only over batch attestations. Published numbers:
   200k+ TPS.
4. **Single channel + namespaces.** Instead of multiple channels, there
   is one channel partitioned into **namespaces**, each with its own
   endorsement policy.

### 3. What is Fabric-Smart-Client

FSC is a Go framework. Each FSC node is a Go process that:

- Holds a Fabric-CA-issued identity belonging to some organisation.
- Runs **views** (Go objects implementing `view.View`) when invoked by
  a CLI client, by another FSC node, or by a registered factory.
- Maintains peer-to-peer sessions to other FSC nodes, used for
  exchanging transaction bodies during endorsement collection.
- Talks to a Fabric / Fabric-X network through driver plugins.

Two pieces of FSC vocabulary you must know cold:

- **View.** A Go struct that implements `Call(viewCtx view.Context) (interface{}, error)`.
  Views are the unit of business logic. There are **initiator** views
  (started by an external trigger — a CLI call, a factory call) and
  **responder** views (started automatically when another FSC node
  initiates a particular initiator view).
- **Topology.** A static description of the network: which Fabric
  organisations exist, which namespaces exist, which FSC nodes exist,
  which org each node belongs to, which view factories the node
  exposes, and for which initiators each node registers responders.

### 4. Why this PoC exists

A developer who has shipped a Fabric chaincode and now needs to migrate
to Fabric-X has to answer one question: *what does my chaincode look
like in the FSC view world?* The answer is non-trivial because:

- A chaincode method is a single function; an FSC view is often a
  multi-actor dance.
- Chaincode reads through `GetState`; FSC reads through the Query
  Service.
- Chaincode events become finality listeners or P2P notifications.
- Chaincode endorsement policies become FSC-node responder topologies
  + namespace policies.

This PoC is a faithful, side-by-side migration of one well-known
chaincode (`asset-transfer-basic`) so a developer can read both and see
the mapping concretely.

---

## Part II — The FSC primitives you need to know

### 5. The View

```go
type View interface {
    Call(viewCtx Context) (interface{}, error)
}
```

That is all. A view is one method. The `viewCtx` parameter is the
runtime — through it the view gets access to the Fabric channel, the
Query Service, sessions to other FSC nodes, the local identity, and so
on.

A view returns a value (anything that's `interface{}`) and an error.
The framework JSON-encodes the return value and ships it back to whoever
invoked the view.

### 6. View factories

A factory creates a view from raw bytes (typically a JSON-encoded input).
Every initiator view has a factory:

```go
type CreateAssetViewFactory struct{}

func (*CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
    f := &CreateAssetView{}
    json.Unmarshal(in, &f.CreateParams)
    return f, nil
}
```

The factory is what gets registered with `RegisterViewFactory("create_asset", ...)`
in the topology. When a CLI does `ii.Client(node).CallView("create_asset", payload)`,
the framework finds the factory by name, calls `NewView(payload)` to
materialise the view, then runs `Call`.

### 7. Initiators vs responders

An **initiator** is a view that starts a flow. It can call
`viewCtx.RunView(...)` to spawn sub-views and can open sessions to
other nodes.

A **responder** is a view that is registered against a particular
initiator type. When node A's initiator opens a session to node B, FSC
automatically starts the responder registered for that initiator on
node B.

In our PoC:

| Initiator               | Responders run on …                          |
| ----------------------- | -------------------------------------------- |
| `InitLedgerView`        | `EndorserView` (endorser), `AuditorView` (auditor) |
| `CreateAssetView`       | same as InitLedger                           |
| `UpdateAssetView`       | same                                          |
| `DeleteAssetView`       | same                                          |
| `TransferAssetView`     | `EndorserView`, `AuditorView`, `TransferAssetReceiverView` (alice or bob, whoever is the new owner) |

Read-only views (Read, Exists, GetAll) have no responders — they don't
build transactions; they just hit the Query Service.

### 8. The state package

`platform/fabric/services/state` is the high-level transaction builder:

```go
tx, _ := state.NewTransaction(viewCtx)   // starts a tx scoped to default channel
tx.SetNamespace("asset-transfer")        // pins the namespace
tx.AddCommand("create")                   // names the operation
tx.AddOutput(asset)                       // write asset to world state
tx.AddInputByLinearID("asset1", in)       // read existing asset (read-set entry)
viewCtx.RunView(state.NewCollectEndorsementsView(tx, signer1, signer2, ...))
viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, timeout))
```

Important details:

- `AddOutput` requires the object to have a `GetLinearID()` method that
  returns the world-state key. Our `Asset.GetLinearID()` returns
  `a.ID`.
- `AddInputByLinearID(id, dst)` loads the current value into `dst`
  *and* records the read-set entry. If the key does not exist, it
  errors out — this is our existence check.
- `AddCommand(name, ids...)` names the operation and optionally lists
  identities involved in it. The endorser (responder) inspects the
  command name to decide which validation logic to run.
- `state.NewCollectEndorsementsView(tx, signers...)` opens sessions to
  each signer in order and runs that signer's responder. Each
  responder either signs (via `state.NewEndorseView`) or returns an
  error which aborts the whole flow.
- `state.NewOrderingAndFinalityWithTimeoutView(tx, timeout)` sends the
  fully signed transaction to the Arma orderer and waits for the
  Fabric-X-Committer to report the transaction as committed.

### 9. The Query Service

`platform/fabricx/core/committer/queryservice` is the read-side. It is
NOT part of the orderer; it is a separate microservice in the
Fabric-X-Committer cluster. The FSC platform exposes it through a thin
Go client:

```go
qs, _ := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
val, _ := qs.GetState("asset-transfer", "asset1")  // single key
out, _ := qs.GetStates(map[Namespace][]PKey{"asset-transfer": {"asset1", "asset2"}}) // batch
```

Key fact (and the PoC's hardest migration moment): **there is no range
query**. The chaincode-era `GetStateByRange("", "")` has no FSC-on-Fabric-X
equivalent. The platform code in `platform/fabricx/core/vault/vault.go`
literally returns `errors.New("GetStateRange not supported by VaultX QueryService")`.

### 10. Finality

After the orderer accepts a transaction, the committer microservices
process it asynchronously. A view that wants to know when a transaction
is committed registers a finality listener:

```go
var wg sync.WaitGroup
wg.Add(1)
_, ch, _ := fabric.GetDefaultChannel(viewCtx)
ch.Committer().AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg))
viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, timeout))
wg.Wait()
```

`OnStatus(ctx, txID, vc, _)` is called with the transaction's
validation code. `fdriver.Valid` means committed-and-applied. The
WaitGroup pattern is how the view blocks until the listener fires.

### 11. Identities

In FSC, identity is an X.509 cert wrapped in `view.Identity`. Every
FSC node has at least one default identity. To address a remote node
in code:

```go
endorserIdentity := ii.Identity("endorser")  // in test code
// or, inside a view, via FSC's network/identity providers
```

`view.Identity` values flow as parameters through `CollectEndorsementsView`
— they tell the framework which sessions to open.

---

## Part III — Walking the PoC, file by file

### 12. `states/asset.go` (~50 lines)

This file defines the on-chain payload. Three things to notice:

```go
type Asset struct {
    AppraisedValue int    `json:"AppraisedValue"`
    Color          string `json:"Color"`
    ID             string `json:"ID"`
    Owner          string `json:"Owner"`
    Size           int    `json:"Size"`
}
```

**Field tags and order match the chaincode verbatim.** This is
intentional. An off-chain indexer that watched the chaincode-era
network can keep working without re-tooling.

```go
func (a *Asset) GetLinearID() (string, error) { return a.ID, nil }
```

**FSC's state package calls this to derive the world-state key.** When
you `tx.AddOutput(asset)`, the framework calls `GetLinearID()` to know
where to store it. That's why every state object needs the method.

```go
func (a *Asset) SetLinearID(id string) string {
    if a.ID == "" { a.ID = id }
    return a.ID
}
```

**Required by `AddInputByLinearID`** when re-loading an existing asset:
the framework reads the world-state key, materialises the struct from
JSON, and tells it what its key was via `SetLinearID`. For us the ID is
always already set, so the function is a no-op except when we ever
build an empty `Asset{}` and load it.

### 13. `views/utils.go` (~50 lines)

Contains `FinalityListener`, the WaitGroup-based bridge between the
committer's status callback and the calling view.

```go
type FinalityListener struct {
    ExpectedTxID string
    ExpectedVC   fdriver.ValidationCode
    WaitGroup    *sync.WaitGroup
}

func (f *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, _ string) {
    if txID == f.ExpectedTxID && vc == f.ExpectedVC {
        f.WaitGroup.Done()
    }
}
```

This is the same pattern used by IOU. In production code you would
also want to handle the **mismatched-vc** case (the listener fired but
the transaction was rejected) by calling `WaitGroup.Done()` on a
companion error channel — but for tutorial purposes, the simple form is
enough.

### 14. `views/init.go` (~130 lines) — InitLedgerView

```go
const Namespace = "asset-transfer"
const FinalityTimeout = 1 * time.Minute
```

These constants are the namespace name and the post-orderer wait time.
We export `Namespace` so other files share it.

```go
type InitParams struct {
    Endorser view.Identity
    Auditor  view.Identity
}
```

**Why pass identities in the input?** Because the initiator does not
know which node is the endorser by name — it needs the identity. The
test harness resolves `ii.Identity("endorser")` to the endorser's
view.Identity and passes it into the view.

The `Call` method:

```go
func (i *InitLedgerView) Call(viewCtx view.Context) (interface{}, error) {
    tx, err := state.NewTransaction(viewCtx)
    tx.SetNamespace(Namespace)
    tx.AddCommand("init")
    for _, asset := range SeedAssets() {
        tx.AddOutput(asset)
    }
    viewCtx.RunView(state.NewCollectEndorsementsView(tx, i.Endorser, i.Auditor))
    // ... finality listener, ordering, wg.Wait()
    return tx.ID(), nil
}
```

**Six AddOutput calls in one transaction.** This is one of the
migration's pleasant moments — chaincode `InitLedger` looped and
called `PutState` six times, generating six separate `PutState` calls
that all had to happen atomically. FSC just batches them in one
transaction body. Atomicity is free.

`SeedAssets()` returns the chaincode's six bootstrap assets verbatim:

```go
{ID: "asset1", Color: "blue",   Size: 5,  Owner: "Tomoko",  AppraisedValue: 300},
{ID: "asset2", Color: "red",    Size: 5,  Owner: "Brad",    AppraisedValue: 400},
// … six entries
```

The bottom of the file also has `EndorserInitView` — a tiny view that
calls `ch.Committer().ProcessNamespace("asset-transfer")`. This is the
endorser-side bootstrap: it tells the FSC node's committer client to
start subscribing to commit events for our namespace. Without it, the
endorser would never see committed transactions.

### 15. `views/create.go` (~95 lines) — CreateAssetView

```go
type CreateParams struct {
    ID, Color, Owner string
    Size, AppraisedValue int
    Endorser, Auditor view.Identity
}

func (c *CreateAssetView) Call(viewCtx view.Context) (interface{}, error) {
    tx, _ := state.NewTransaction(viewCtx)
    tx.SetNamespace(Namespace)
    tx.AddCommand("create")
    tx.AddOutput(&states.Asset{
        ID: c.ID, Color: c.Color, Size: c.Size,
        Owner: c.Owner, AppraisedValue: c.AppraisedValue,
    })
    viewCtx.RunView(state.NewCollectEndorsementsView(tx, c.Endorser, c.Auditor))
    // ... finality, return tx.ID()
}
```

**Notice no existence check at the initiator side.** The chaincode
version did `AssetExists` then `PutState`; we delegate the existence
check to the endorser. Why? Because the endorser is the one whose
signature is enforced by the namespace policy — its check is the one
that *matters* for security. Doing the same check at the initiator
would just be belt-and-suspenders. The endorser uses the Query Service
to fetch the would-be-overwritten key.

### 16. `views/read.go` (~70 lines) — ReadAssetView

```go
func (r *ReadAssetView) Call(viewCtx view.Context) (interface{}, error) {
    network, ch, _ := fabric.GetDefaultChannel(viewCtx)
    qs, _ := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
    val, _ := qs.GetState(Namespace, r.ID)
    if val == nil {
        return nil, errors.Errorf("the asset %s does not exist", r.ID)
    }
    asset := &states.Asset{}
    json.Unmarshal(val.Raw, asset)
    return asset, nil
}
```

**Notice what is missing.** No `state.NewTransaction`, no command, no
endorsement, no orderer, no finality. A read-only view is just a
function call into the Query Service. This is the cleanest migration
shape — every chaincode method that did nothing but `GetState` becomes
this short.

The error string `"the asset %s does not exist"` is taken verbatim
from the chaincode for compatibility with clients that pattern-match
on it.

### 17. `views/exists.go` (~50 lines) — AssetExistsView

Identical shape to ReadAssetView except we just return the boolean
without unmarshalling. The chaincode `AssetExists` returned `(bool, error)`;
ours returns the same.

### 18. `views/update.go` (~95 lines) — UpdateAssetView

```go
func (u *UpdateAssetView) Call(viewCtx view.Context) (interface{}, error) {
    tx, _ := state.NewTransaction(viewCtx)
    tx.SetNamespace(Namespace)
    tx.AddCommand("update")

    in := &states.Asset{}
    tx.AddInputByLinearID(u.ID, in)  // read existing → read-set entry
    out := &states.Asset{
        ID: u.ID, Color: u.Color, Size: u.Size,
        Owner: u.Owner, AppraisedValue: u.AppraisedValue,
    }
    tx.AddOutput(out)               // overwrite

    // collect, order, wait for finality
}
```

**`AddInputByLinearID` is doing two jobs:**

1. It loads the current asset into `in`. If the asset does not exist,
   the call errors and we abort early — that's our existence check.
2. It records the read-set entry. The committer will reject the
   transaction at validation time if anyone modified the same key
   between our read and our submission (this is Fabric's
   read-version-check, preserved in Fabric-X).

The shape `read existing → produce updated → submit` is the standard
optimistic-concurrency pattern. If a concurrent transaction wins the
race, our transaction is invalidated by the committer and we get a
non-`Valid` finality code.

### 19. `views/delete.go` (~80 lines) — DeleteAssetView

```go
tx.AddCommand("delete")
tx.AddInputByLinearID(d.ID, in)  // existence check + read-set
// no AddOutput at the same key → world-state delete
```

**The "missing AddOutput" is the deletion.** When the committer
applies a transaction's writes, it processes inputs and outputs as a
single key-value diff. An input without a matching output in the
transaction's outputs map is interpreted as a delete.

### 20. `views/transfer.go` (~140 lines) — TransferAssetView

This is the architecturally richest view. It introduces three things
no chaincode method has:

1. **Receiver acceptance.** `TransferAssetReceiverView` is registered
   on the new owner's FSC node. The new owner's signature is collected
   via `CollectEndorsementsView`. If the new owner refuses, the entire
   flow aborts.
2. **Pure-transfer invariant.** The endorser checks that Color, Size,
   and AppraisedValue do not change. This is impossible to enforce in
   classical chaincode without re-reading the asset and comparing in
   the chaincode body — and even then, a buggy chaincode could be
   bypassed by a buggy peer.
3. **Initiator-side fast-fail on no-op.** We assert
   `in.Owner != t.NewOwnerLabel` *before* opening sessions, saving the
   network round-trip for an obviously-invalid transfer.

Reading the receiver:

```go
type TransferAssetReceiverView struct{}

func (r *TransferAssetReceiverView) Call(viewCtx view.Context) (interface{}, error) {
    tx, _ := state.ReceiveTransaction(viewCtx)
    // inspect the proposed new state
    out := &states.Asset{}
    tx.GetOutputAt(0, out)
    // accept-or-reject decision could go here
    viewCtx.RunView(state.NewEndorseView(tx))   // sign
    return viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, time.Minute))
}
```

`state.ReceiveTransaction(viewCtx)` is the responder's first action —
it blocks until the initiator sends us the assembled transaction. After
inspection, `state.NewEndorseView(tx)` produces the signature.
`state.NewFinalityWithTimeoutView(tx, ...)` blocks until the receiver
has seen the transaction commit; this is what lets the receiver kick
off post-receipt logic (notifications, local cache update, etc.) at
exactly the right moment.

### 21. `views/get_all.go` (~95 lines) — GetAllAssetsView

The most-discussed view in the PoC. Three substantive comments to make:

```go
keys := make([]driver.PKey, len(g.IDs))
for i, id := range g.IDs { keys[i] = id }
out, _ := qs.GetStates(map[driver.Namespace][]driver.PKey{Namespace: keys})
```

**`GetStates` is one round trip.** The Query Service supports batch
GET via a multi-key map. Don't do `for id := range ids { qs.GetState(...) }` —
that's len(ids) round trips for no reason.

The view's godoc spells out three migration patterns for the larger
question of *how do we replace `GetStateByRange`*:

1. Explicit-ID-list (this implementation).
2. On-chain index key.
3. Off-chain index built from finality listeners.

Read the comments in the file — they are intentionally long because the
sharp edge they describe is the single most asked migration question.

### 22. `views/endorser.go` (~145 lines) — EndorserView

This file is the chaincode replacement. Reading top-to-bottom:

```go
tx, _ := state.ReceiveTransaction(viewCtx)
assert.Equal(1, len(tx.Namespaces()))
assert.Equal(Namespace, tx.Namespaces()[0])
assert.Equal(1, tx.Commands().Count())
qs, _ := queryservice.GetQueryService(...)
switch cmd := tx.Commands().At(0); cmd.Name {
case "init":   /* per-command checks */
case "create": /* … */
case "update": /* … */
case "delete": /* … */
case "transfer": /* … */
}
viewCtx.RunView(state.NewEndorseView(tx))   // sign
```

Each `case` enforces the invariants that the corresponding chaincode
method enforced. The mapping is exact:

- `case "create"` runs `AssetExists` via `qs.GetState(Namespace, out.ID)`
  and rejects if non-nil — same as chaincode's `s.AssetExists` ahead
  of `PutState`.
- `case "update"` checks `in.ID == out.ID` and that no field that
  shouldn't change has changed.
- `case "transfer"` enforces the Color/Size/AppraisedValue invariant
  (a strictly tighter check than chaincode performed).

**The endorser does not run the actual write.** The committer does,
after it validates the transaction against the namespace policy. The
endorser only signs to say "I attest that this transaction's body is
internally consistent and would pass my view of the world state."

### 23. `views/auditor.go` (~60 lines) — AuditorView

A minimalist responder. It logs what it saw and signs:

```go
logger.Infof("AUDIT: tx=%s namespace=%s command=%s inputs=%d outputs=%d", ...)
viewCtx.RunView(state.NewEndorseView(tx))
return nil, nil
```

Because the auditor is in Org1 (same as the endorser) but does NOT have
the approver role, its signature is collected by the initiator's
`CollectEndorsementsView` but is not strictly required by the namespace
policy. In a stricter topology you would either:

- Give the auditor the approver role too and require unanimity-of-Org1,
  so the auditor's signature is hard-required; or
- Add the auditor's organisation to the namespace policy as a separate
  approver org.

We chose the soft form for tutorial simplicity.

### 24. `topology.go` (~125 lines) — Topology

The most important file for understanding network shape.

```go
fabricTopology := nwofabricx.NewDefaultTopology()
fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
fabricTopology.SetNamespaceApproverOrgs("Org1")
fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1")
```

These four lines:
1. Create a default Fabric-X topology (orderer config, MSPs, etc.).
2. Declare three orgs.
3. Say only Org1 can approve namespaces.
4. Create the `asset-transfer` namespace whose policy is "all of Org1".

Then the FSC topology declares each node:

```go
fscTopology.AddNodeByName(EndorserNode).
    AddOptions(fabric.WithOrganization("Org1")).
    AddOptions(scv2.WithApproverRole()).
    RegisterResponder(&views.EndorserView{}, &views.InitLedgerView{}).
    RegisterResponder(&views.EndorserView{}, &views.CreateAssetView{}).
    /* … one Register per state-changing initiator … */
    RegisterViewFactory("init", &views.EndorserInitViewFactory{})
```

`scv2.WithApproverRole()` is the option that says "this node can sign
on behalf of its org for namespace approval". Without it, an Org1 node
exists but cannot endorse.

`RegisterResponder(&EndorserView{}, &InitLedgerView{})` reads as:
"whenever some node initiates `InitLedgerView`, my responder for that
flow is `EndorserView`."

`RegisterViewFactory("init", &EndorserInitViewFactory{})` lets a CLI
client invoke the bootstrap view by string name `"init"`.

### 25. `sdk.go` (~45 lines)

A thin wrapper around `fabricx.NewSDK`. The job is to register
`state.VaultService` so that views which read state directly via the
vault (not the Query Service) work. Mirrors `iou/sdk.go` exactly.

### 26. `commands_test.go` (~170 lines)

Test helpers, one per chaincode method. Each function is a short
wrapper around `ii.Client(node).CallViewWithContext(ctx, factoryName, payload)`.

Reading one:

```go
func createAsset(ii *integration.Infrastructure, initiator string, a *states.Asset) {
    ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
    defer cancel()
    _, err := ii.Client(initiator).CallViewWithContext(ctx, "create_asset",
        common.JSONMarshall(views.CreateParams{
            ID: a.ID, Color: a.Color, Size: a.Size,
            Owner: a.Owner, AppraisedValue: a.AppraisedValue,
            Endorser: ii.Identity(chaincodetofsc.EndorserNode),
            Auditor:  ii.Identity(chaincodetofsc.AuditorNode),
        }))
    Expect(err).NotTo(HaveOccurred())
}
```

Three things to notice:

1. `ii.Client(initiator)` returns a stub talking to the named FSC node.
2. The string `"create_asset"` is the factory name from the topology.
3. `ii.Identity("endorser")` resolves the endorser node's `view.Identity`
   so it can be embedded in the input payload.

### 27. `chaincode_to_fsc_test.go` (~120 lines)

The single integration test. Each `By(...)` block names the chaincode
method being demonstrated. Reading the test top-to-bottom is the same
as reading the chaincode top-to-bottom — that's the design.

---

## Part IV — Tracing one full transaction

Let me walk through what happens when the test calls `createAsset(s.II, "issuer", asset7)`.

### 28. The CreateAsset trace, end to end

1. **Test side.** `ii.Client("issuer").CallViewWithContext(ctx, "create_asset", payload)`.
   The integration framework finds the issuer FSC node, opens a gRPC
   stream to its view-service endpoint, and sends `(factory="create_asset", input=payload)`.

2. **Issuer node receives.** The view dispatcher looks up the factory
   `"create_asset"` and finds `CreateAssetViewFactory`. It calls
   `NewView(payload)` which JSON-decodes payload into `CreateParams`
   and returns a `CreateAssetView`.

3. **`Call(viewCtx)` runs on the issuer.** Steps in order:
   - `state.NewTransaction(viewCtx)` allocates a fresh transaction
     scoped to the default channel.
   - `tx.SetNamespace("asset-transfer")` and `tx.AddCommand("create")`.
   - `tx.AddOutput(asset)` records the would-be write.
   - `state.NewCollectEndorsementsView(tx, endorser, auditor)` opens
     two sessions: one to the endorser node (Org1, approver role), one
     to the auditor node (Org1, no approver role).

4. **Endorser node receives session.** FSC's responder dispatcher sees
   "session is for `CreateAssetView`" and starts the registered responder
   `EndorserView`. The endorser's `Call`:
   - `state.ReceiveTransaction(viewCtx)` reads the transaction body
     from the session.
   - Validates: 1 namespace, 1 command, command is "create", 0 inputs,
     1 output, output ID is non-empty.
   - Runs `qs.GetState(Namespace, out.ID)` against the Fabric-X-Committer
     Query Service. Returns nil → asset doesn't exist → OK.
   - Runs `state.NewEndorseView(tx)` which signs the transaction and
     sends the signature back over the session.

5. **Auditor node receives session.** Same dispatcher pattern. Its
   responder `AuditorView`:
   - `state.ReceiveTransaction(viewCtx)`.
   - Logs an `AUDIT:` line.
   - Signs and returns.

6. **Issuer's `CollectEndorsementsView` returns.** The transaction now
   has signatures from issuer (self), endorser, auditor.

7. **Issuer registers a finality listener** on its local committer
   client for `tx.ID()`.

8. **`state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout)`**:
   - Sends the fully-signed transaction to the Arma orderer's router.
   - Returns when the committer reports finality.

9. **Arma orderer side.**
   - Router receives transaction.
   - A batcher persists it.
   - Consenters run BFT consensus over the batch attestation.
   - Assembler emits the totally-ordered block to the Sidecar.

10. **Fabric-X-Committer side.**
    - Sidecar pushes the block into Coordinator.
    - Verification Service checks signatures (all three are present
      and valid against the namespace policy: Org1 unanimity is
      satisfied because the endorser's signature alone is sufficient).
    - Validator-Committer applies the write-set to the world-state DB
      and emits a per-tx status event.

11. **Issuer's finality listener fires.** `OnStatus(_, txID, fdriver.Valid, _)`
    is called. The WaitGroup is `Done`'d. The view's `wg.Wait()` returns.

12. **`Call` returns `tx.ID()`.** The framework JSON-encodes and sends
    it back over the gRPC stream to the test.

13. **Test sees `nil` error.** `Expect(err).NotTo(HaveOccurred())` passes.

That's the full trace. Twelve hops, and it usually completes in a few
seconds on a laptop.

### 29. The TransferAsset trace differs in one place

Step 3 above changes:

```go
state.NewCollectEndorsementsView(tx, t.NewOwner, t.Endorser, t.Auditor)
```

Three signers, in this order: **new owner first**, endorser, auditor.
FSC opens three sessions in order. The new owner runs
`TransferAssetReceiverView`, which inspects the proposed new asset and
either accepts (signs) or returns an error. If it returns an error, the
session error propagates back to the initiator's
`CollectEndorsementsView`, which returns the error to the initiator's
`Call`, which returns the error to the test.

That's it. Receiver acceptance is not a special framework feature —
it's just what happens when a responder returns an error before
signing.

---

## Part V — Common mistakes and how to avoid them

### 30. "expected one namespace, got 0" at the endorser

You forgot `tx.SetNamespace(Namespace)` in the initiator. Always set
the namespace before adding commands or outputs.

### 31. The transaction times out at `wg.Wait()`

Three usual causes:

- **The endorser never runs `EndorserInitView`.** Without it, the
  endorser's committer client never subscribes to namespace events, so
  the finality listener fires for the issuer but the endorser never
  hears the commit. Run `initEndorser(s.II)` first.
- **The namespace policy is unsatisfied.** Check `tx.Namespaces()` and
  the topology's `AddNamespaceWithUnanimity` line — if you wrote
  `AddNamespaceWithUnanimity("AssetTransfer", "Org1")` (capital A) but
  the views use `Namespace = "asset-transfer"`, the names don't match
  and the policy says reject.
- **The orderer is not running.** Most often a `docker compose` step
  failed silently. Check `docker ps` for the Fabric-X orderer
  containers.

### 32. "Unknown command 'create'" at the endorser

You added a new command to the initiator without adding the matching
case to `EndorserView`. Each command name must be handled in the switch
in `views/endorser.go`.

### 33. "asset NOT FOUND" on read after create

The committer is asynchronous. `wg.Wait()` ensures the listener fired,
but the listener fires when the validator-committer applies the
write-set to its local DB. The Query Service reads from a *different*
internal store. There can be a tiny lag. In practice the test passes
because the lag is microseconds, but if you ever see flakes here the
fix is to retry the read with a short sleep.

### 34. The receiver responder is not invoked

Two possibilities:

- The initiator did not list the new owner as a signer in
  `CollectEndorsementsView(tx, t.NewOwner, ...)`.
- The new owner's FSC node did not register the receiver responder
  with `RegisterResponder(&views.TransferAssetReceiverView{}, &views.TransferAssetView{})`.

### 35. "AddInputByLinearID: …does not exist"

Trying to update or delete an asset that was never created. This is the
existence check working as designed. Catch the error and return a
client-friendly message.

---

## Part VI — Extending the PoC

### 36. Adding a new chaincode method, by analogy

Suppose you want to add `RenameAsset(id, newColor)`. Steps:

1. **Add a view file** `views/rename.go` with `RenameAssetView` modelled
   after `UpdateAssetView`. It should `AddInputByLinearID`, mutate
   `Color`, `AddOutput`. The command name might be `"rename"`.
2. **Add a case to the endorser.** In `views/endorser.go`, add
   `case "rename":` with the invariants you want (Color must change,
   no other field changes).
3. **Register the factory in `topology.go`.** Add
   `.RegisterViewFactory("rename_asset", &views.RenameAssetViewFactory{})`
   on the alice and bob nodes.
4. **Register the endorser responder in `topology.go`.** Add
   `.RegisterResponder(&views.EndorserView{}, &views.RenameAssetView{})`
   on the endorser node and the auditor node.
5. **Add a helper in `commands_test.go`.** `func renameAsset(...)`
   modelled after `updateAsset`.
6. **Add a `By(...)` block in the integration test** exercising the
   happy path and at least one negative path.

That's the whole pattern. Every new chaincode method is six edits.

### 37. Adding a new actor (e.g. a regulator)

1. Define `RegulatorNode = "regulator"` and add it in `topology.go`
   with whichever org you want.
2. Decide whether the regulator is an approver (gets its sig
   hard-required by the namespace policy) or a witness (collected by
   the initiator, not enforced by policy).
3. Register a `RegulatorView` responder on every initiator the
   regulator should observe.
4. Pass `ii.Identity("regulator")` into every initiator's input as a
   new field.

### 38. Wiring a private-data demo

Take the `transfer.go` flow as a template, then:

1. Add a private-payload struct that does NOT go on chain (e.g.
   `TransferAgreement{...}`).
2. The initiator computes `hash := sha256(JSON(agreement))`, sends the
   full agreement to the receiver via `viewCtx.GetSession`, and
   includes only `hash` as an `AddOutput`.
3. The receiver verifies the hash and then signs.
4. The endorser verifies that the on-chain output's hash is well-formed
   but does not see the agreement itself.

This is migration pattern #2 from the README's §9.4.

---

## Part VII — Glossary

- **Arma.** The Fabric-X BFT orderer (paper at arxiv.org/abs/2405.16575).
- **Approver role.** An option (`scv2.WithApproverRole()`) that lets an
  FSC node sign for namespace approval on behalf of its org.
- **Command.** A named operation attached to a transaction (`tx.AddCommand(name, ids...)`).
  The endorser uses the name to dispatch to the right validation logic.
- **Committer.** The Fabric-X-Committer microservice cluster: Sidecar,
  Coordinator, Verification Service, Validator-Committer, Query
  Service. Replaces the classical-Fabric peer's commit phase.
- **Factory.** An object that materialises a view from raw bytes.
  Registered with `RegisterViewFactory(name, factory)`.
- **Finality listener.** A callback that fires when a transaction
  reaches a particular validation code.
- **FSC node.** A Go process running the FSC framework. It holds an
  identity, opens P2P sessions, runs views.
- **Initiator.** A view that starts a flow; usually invoked by a CLI or
  factory call.
- **Namespace.** A Fabric-X partition of the channel with its own
  endorsement policy. Replaces classical-Fabric channels.
- **NWO.** "Network Operator", FSC's integration test framework that
  spins up real Fabric / Fabric-X networks for tests.
- **Query Service.** The Fabric-X-Committer's read-side microservice.
  Replaces `ctx.GetStub().GetState(...)`.
- **Responder.** A view registered against an initiator, automatically
  started when another FSC node opens a session for that initiator.
- **Session.** A bidirectional message channel between two FSC nodes
  during a flow, opened by the initiator's
  `CollectEndorsementsView` (and other view-runner helpers).
- **State.** A Go struct that satisfies `GetLinearID()` and is stored
  on chain via `tx.AddOutput(state)`.
- **Topology.** A static description of the network used by NWO to
  bring up Fabric / Fabric-X + FSC nodes for a test.
- **Transaction.** A `state.Tx` object built by `state.NewTransaction(viewCtx)`,
  fed to `CollectEndorsementsView` and `OrderingAndFinalityView`.
- **View.** A Go struct implementing `Call(viewCtx) (interface{}, error)`.
  The unit of business logic in FSC.
- **View context.** The `view.Context` argument to `Call`. It exposes
  the network, sessions, identities, and the local SDK.

---

## Part VIII — Suggested reading order

If you have not used FSC at all, read in this order:

1. This file's Part I and Part II (background + primitives).
2. `states/asset.go` — the simplest file in the PoC.
3. `views/utils.go`, `views/read.go`, `views/exists.go` — read-only
   shapes; no transactions.
4. `views/init.go` — your first state-changing view; introduces
   `AddOutput`, `AddCommand`, `CollectEndorsements`, finality.
5. `views/create.go`, `views/update.go`, `views/delete.go` — variations
   on the same pattern.
6. `views/endorser.go` — the chaincode replacement; the most important
   file in the PoC.
7. `views/transfer.go` — receiver acceptance; the most architecturally
   interesting file.
8. `views/get_all.go` — the migration's sharp edge.
9. `topology.go` — how the pieces connect.
10. `chaincode_to_fsc_test.go` — runnable proof everything works.

Then come back to this file's Part IV (the trace) and read it with the
code open in another window. After that you should be able to read any
FSC integration test in the FSC repo without help.

---

## Part IX — Where to ask when you're stuck

- **FSC code.** Open the same path in
  [hyperledger-labs/fabric-smart-client](https://github.com/hyperledger-labs/fabric-smart-client).
  The integration tests under `integration/fabricx/` are the canonical
  reference implementations.
- **Fabric-X.** [hyperledger/fabric-x](https://github.com/hyperledger/fabric-x)
  has the architecture diagrams and version compatibility matrix.
- **The mentors.** Marcus Brandenburger (mbrandenburger), Maria Munaro
  (munapower), Samuel Venzi (samuelvenzi) — all named on the mentorship
  issue [LF-Decentralized-Trust-Mentorships/mentorship-program#59](https://github.com/LF-Decentralized-Trust-Mentorships/mentorship-program/issues/59).
- **LFDT Discord** — `#fabric-smart-client` and `#fabric-x` channels.

End of walkthrough.
