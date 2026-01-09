# TokenX Code Walkthrough

This document provides a detailed walkthrough of the TokenX codebase, explaining how each component works and how they interact.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Code Flow: Issue Tokens](#code-flow-issue-tokens)
3. [Code Flow: Transfer Tokens](#code-flow-transfer-tokens)
4. [Code Flow: Endorsement Process](#code-flow-endorsement-process)
5. [Key Components Deep Dive](#key-components-deep-dive)
6. [FabricX Core Layer](#fabricx-core-layer)
7. [Common Patterns](#common-patterns)
8. [Debugging Guide](#debugging-guide)

---

## Architecture Overview

### Layer Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Integration Tests                              │
│  tokenx_test.go - Ginkgo test suite                                 │
└────────────────────────────────────────────────────────────────────▼┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         Views Layer                                  │
│  views/issue.go, transfer.go, approver.go, etc.                     │
│  Business logic for token operations                                 │
└────────────────────────────────────────────────────────────────────▼┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    State Transaction Layer                           │
│  platform/fabric/services/state/                                     │
│  Wraps transactions, manages RWSet                                   │
└────────────────────────────────────────────────────────────────────▼┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Endorser Layer                                  │
│  platform/fabric/services/endorser/                                  │
│  Collects endorsements, submits to ordering                          │
└────────────────────────────────────────────────────────────────────▼┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      FabricX Core Layer                              │
│  platform/fabricx/core/                                              │
│  transaction/, vault/, finality/, committer/                         │
│  FabricX-specific implementation with counter-based versioning       │
└────────────────────────────────────────────────────────────────────▼┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   Fabric-X Committer Sidecar                         │
│  Docker container: hyperledger/fabric-x-committer-test-node:0.1.5   │
│  Handles ordering, validation, commitment                            │
└─────────────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
integration/fabricx/tokenx/
├── tokenx_test.go      # Ginkgo test suite entry point
├── topology.go         # Network topology (nodes, organizations)
├── sdk.go              # SDK initialization
├── states/
│   └── states.go       # Data models (Token, TransactionRecord)
└── views/
    ├── issue.go        # IssueView - create new tokens
    ├── transfer.go     # TransferView - send tokens
    ├── redeem.go       # RedeemView - burn tokens
    ├── swap.go         # SwapView - atomic exchanges
    ├── balance.go      # BalanceView - query holdings
    ├── approver.go     # ApproverView - validate & endorse
    └── utils.go        # Helpers (FinalityListener)
```

---

## Code Flow: Issue Tokens

### Sequence Diagram

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│   Test   │     │  Issuer  │     │ Approver │     │ Sidecar  │
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │
     │ CallView       │                │                │
     │ ("issue")      │                │                │
     │───────────────▶│                │                │
     │                │                │                │
     │                │ Create Token   │                │
     │                │ Add to RWSet   │                │
     │                │────────────────│                │
     │                │                │                │
     │                │ Collect        │                │
     │                │ Endorsements   │                │
     │                │───────────────▶│                │
     │                │                │                │
     │                │                │ Validate       │
     │                │                │ Add signature  │
     │                │◀───────────────│                │
     │                │                │                │
     │                │ Add Finality   │                │
     │                │ Listener       │                │
     │                │────────────────│                │
     │                │                │                │
     │                │ Submit to      │                │
     │                │ Ordering       │                │
     │                │───────────────────────────────▶│
     │                │                │                │
     │                │                │                │ Validate
     │                │                │                │ Commit
     │                │                │                │
     │                │ Finality       │                │
     │                │ Notification   │                │
     │                │◀───────────────────────────────│
     │                │                │                │
     │ Token ID       │                │                │
     │◀───────────────│                │                │
     │                │                │                │
```

### Code Walkthrough

#### Step 1: Test calls IssueView

**File:** `tokenx_test.go`

```go
func IssueTokens(ii *integration.Infrastructure, tokenType string, amount uint64, recipient string) string {
    res, err := ii.Client(tokenx.IssuerNode).CallView("issue", common.JSONMarshall(&views.Issue{
        TokenType: tokenType,
        Amount:    amount,
        Recipient: ii.Identity(recipient),
        Approvers: []view.Identity{ii.Identity(tokenx.ApproverNode)},
    }))
    // ...
}
```

#### Step 2: IssueView.Call() executes

**File:** `views/issue.go`

```go
func (i *IssueView) Call(ctx view.Context) (interface{}, error) {
    // 1. Create token state
    token := &states.Token{
        Type:     i.TokenType,
        Amount:   i.Amount,
        Owner:    i.Recipient,
        IssuerID: "issuer",
    }
    
    // 2. Create new transaction
    tx, err := state.NewTransaction(ctx)
    tx.SetNamespace(TokenxNamespace)  // "tokenx"
    
    // 3. Add command and output
    tx.AddCommand("issue")
    token.LinearID = tx.ID()
    tx.AddOutput(token)
    
    // 4. Collect endorsements from approvers
    ctx.RunView(state.NewCollectEndorsementsView(tx, i.Approvers...))
    
    // 5. Set up finality listener
    lm, _ := finality.GetListenerManager(ctx, network.Name(), ch.Name())
    var wg sync.WaitGroup
    wg.Add(1)
    lm.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg))
    
    // 6. Submit to ordering
    ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout))
    
    // 7. Wait for finality notification
    wg.Wait()
    
    return token.LinearID, nil
}
```

#### Step 3: ApproverView validates and endorses

**File:** `views/approver.go`

```go
func (a *ApproverView) Call(ctx view.Context) (interface{}, error) {
    // 1. Receive transaction from initiator
    session, tx, _ := state.ReceiveTransaction(ctx)
    
    // 2. Validate the transaction
    // - Check commands are valid
    // - Check token amounts are positive
    // - Check business rules
    
    // 3. Endorse the transaction
    // This creates a ProposalResponse with the approver's signature
    session.Send(tx.Bytes())  // Send back endorsement
    
    return nil, nil
}
```

---

## Code Flow: Transfer Tokens

### Key Difference from Issue

Transfer involves:
1. **Input token** - consumed (deleted from ledger)
2. **Output token(s)** - created
   - One for recipient (transfer amount)
   - One for sender as "change" (if partial transfer)

### Partial Transfer Example

```
Input:  Token(Alice, 1000 USD)
Transfer: 300 USD to Bob

Outputs:
  - Token(Bob, 300 USD)     # New token for recipient
  - Token(Alice, 700 USD)   # Change token for sender
```

**File:** `views/transfer.go`

```go
func (t *TransferView) Call(ctx view.Context) (interface{}, error) {
    // 1. Load existing token from vault
    token, _ := vault.GetState(...)
    
    // 2. Validate transfer
    if t.Amount > token.Amount {
        return nil, errors.New("insufficient balance")
    }
    
    // 3. Create output tokens
    // Recipient token
    recipientToken := &states.Token{
        Type:   token.Type,
        Amount: t.Amount,
        Owner:  t.Recipient,
    }
    tx.AddOutput(recipientToken)
    
    // 4. Create change token if partial transfer
    if t.Amount < token.Amount {
        changeToken := &states.Token{
            Type:   token.Type,
            Amount: token.Amount - t.Amount,
            Owner:  token.Owner,  // Back to sender
        }
        tx.AddOutput(changeToken)
    }
    
    // 5. Mark input as consumed
    tx.Delete(token)
    
    // 6. Collect endorsements, submit, wait for finality
    // ... (same pattern as issue)
}
```

---

## Code Flow: Endorsement Process

This is the most critical flow where the bug was found and fixed.

### How Endorsement Collection Works

**File:** `platform/fabric/services/endorser/endorsement.go`

```go
func (c *collectEndorsementsView) Call(ctx view.Context) (interface{}, error) {
    // 1. Get the transaction's current results (RWSet bytes)
    res := c.tx.Results()  // Issuer's serialized RWSet
    
    // 2. For each party that needs to endorse
    for _, party := range c.parties {
        // Send transaction to the party
        session.Send(c.tx.Bytes())
        
        // Receive proposal response
        proposalResponse := &pb.ProposalResponse{}
        session.Receive(&proposalResponse)
        
        // 3. CRITICAL: Compare results
        if !bytes.Equal(res, proposalResponse.Results()) {
            // THE BUG WAS HERE!
            // If results don't match, endorsement fails
            return nil, errors.New("received different results")
        }
        
        // 4. Store endorsement
        c.tx.AppendProposalResponse(proposalResponse)
    }
}
```

### The Bug: Namespace Version Mismatch

**Problem Location:** `platform/fabricx/core/transaction/transaction.go`

```go
func (t *Transaction) getProposalResponse(signer SerializableSigner) (*pb.ProposalResponse, error) {
    // BEFORE FIX: Always re-serialized RWSet
    rwset, _ := t.GetRWSet()
    rawTx, _ := rwset.Bytes()  // <-- Uses local namespace versions!
    
    // AFTER FIX: Use received bytes if available
    var rawTx []byte
    if len(t.RWSet) != 0 {
        rawTx = t.RWSet  // Use issuer's original bytes
    } else {
        rwset, _ := t.GetRWSet()
        rawTx, _ = rwset.Bytes()
    }
}
```

### Why Namespace Versions Differ

**File:** `platform/fabricx/core/vault/interceptor.go`

```go
func (rws *Interceptor) namespaceVersions() map[string]uint64 {
    versions := make(map[string]uint64)
    
    // Reads _meta namespace from LOCAL vault
    // Different nodes have different versions!
    rwsIt, _ := rws.wrapped.NamespaceReads("_meta")
    for rwsIt.HasNext() {
        rw, _ := rwsIt.Next()
        versions[rw.Namespace] = rw.Block  // Local block number
    }
    
    return versions
}
```

---

## Key Components Deep Dive

### States (Data Models)

**File:** `states/states.go`

```go
// Token represents a fungible token (UTXO model)
type Token struct {
    Type      string        `json:"type"`       // e.g., "USD", "EUR"
    Amount    uint64        `json:"amount"`     // 8 decimal places
    Owner     view.Identity `json:"owner"`      // Current owner
    LinearID  string        `json:"linear_id"`  // Unique ID
    IssuerID  string        `json:"issuer_id"`  // Original issuer
    CreatedAt time.Time     `json:"created_at"`
}

// GetLinearID implements state.LinearState
func (t *Token) GetLinearID() (string, error) {
    // Creates composite key: TKN:linearID
    return rwset.CreateCompositeKey(TypeToken, []string{t.LinearID})
}
```

### FinalityListener

**File:** `views/utils.go`

```go
type FinalityListener struct {
    ExpectedTxID string
    ExpectedVC   fdriver.ValidationCode
    WaitGroup    *sync.WaitGroup
}

func (f *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, message string) {
    if txID == f.ExpectedTxID && vc == f.ExpectedVC {
        time.Sleep(5 * time.Second)  // Delay for state propagation
        f.WaitGroup.Done()           // Unblock the waiting view
    }
}
```

### Topology Setup

**File:** `topology.go`

```go
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions, owners ...string) []api.Topology {
    // 1. Create FabricX topology
    fabricTopology := nwofabricx.NewDefaultTopology()
    fabricTopology.AddOrganizationsByName("Org1")
    fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1")
    
    // 2. Create FSC topology
    fscTopology := fsc.NewTopology()
    
    // 3. Add approver node (has endorsement power)
    fscTopology.AddNodeByName(ApproverNode).
        AddOptions(fabric.WithOrganization("Org1")).
        AddOptions(scv2.WithApproverRole()).  // KEY: Marks as approver
        RegisterResponder(&tokenviews.ApproverView{}, &tokenviews.IssueView{})
    
    // 4. Add issuer node
    fscTopology.AddNodeByName(IssuerNode).
        AddOptions(fabric.WithOrganization("Org1")).
        RegisterViewFactory("issue", &tokenviews.IssueViewFactory{})
    
    // 5. Add owner nodes
    for _, ownerName := range owners {
        fscTopology.AddNodeByName(ownerName).
            RegisterViewFactory("query", &tokenviews.BalanceViewFactory{})
    }
    
    return []api.Topology{fabricTopology, fscTopology}
}
```

---

## FabricX Core Layer

### Transaction Management

**File:** `platform/fabricx/core/transaction/transaction.go`

Key methods:
- `NewTransaction()` - Creates new transaction
- `GetRWSet()` - Returns read-write set
- `Results()` - Serializes RWSet for endorsement comparison
- `getProposalResponse()` - Creates ProposalResponse (where fix was applied)

### Vault Interceptor

**File:** `platform/fabricx/core/vault/interceptor.go`

Key methods:
- `Bytes()` - Serializes RWSet including namespace versions
- `namespaceVersions()` - Reads version info from local vault

### Finality Service

**File:** `platform/fabricx/core/finality/nlm.go`

Key components:
- `notificationListenerManager` - Manages finality listeners
- `AddFinalityListener()` - Registers callback for transaction status
- `listen()` - Maintains gRPC stream with sidecar

---

## Common Patterns

### Pattern 1: View with Endorsement and Finality

All transaction-creating views follow this pattern:

```go
func (v *SomeView) Call(ctx view.Context) (interface{}, error) {
    // 1. Create transaction
    tx, _ := state.NewTransaction(ctx)
    tx.SetNamespace("...")
    tx.AddCommand("...")
    
    // 2. Add inputs/outputs
    tx.AddOutput(someState)
    
    // 3. Collect endorsements
    ctx.RunView(state.NewCollectEndorsementsView(tx, approvers...))
    
    // 4. Set up finality listener
    lm, _ := finality.GetListenerManager(ctx, network, channel)
    var wg sync.WaitGroup
    wg.Add(1)
    lm.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg))
    
    // 5. Submit to ordering
    ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, timeout))
    
    // 6. Wait for finality
    wg.Wait()
    
    return result, nil
}
```

### Pattern 2: Approver Responder

```go
func (a *ApproverView) Call(ctx view.Context) (interface{}, error) {
    // 1. Receive transaction
    session, tx, _ := state.ReceiveTransaction(ctx)
    
    // 2. Validate (business rules)
    if err := validate(tx); err != nil {
        return nil, err
    }
    
    // 3. Send endorsement
    return session.Send(tx.Bytes())
}
```

### Pattern 3: State Queries

```go
func (b *BalanceView) Call(ctx view.Context) (interface{}, error) {
    // Get vault service
    vs, _ := ctx.GetService(state.VaultService{})
    vault := vs.Vault(channel)
    
    // Query by composite key prefix
    it, _ := vault.GetStateByPartialCompositeKey("TKN", []string{})
    
    var tokens []*Token
    for it.HasNext() {
        entry, _ := it.Next()
        token := &Token{}
        json.Unmarshal(entry.Raw, token)
        tokens = append(tokens, token)
    }
    
    return tokens, nil
}
```

---

## Debugging Guide

### 1. Test Hangs / Sidecar Issues
**Symptom**: Test hangs at `Post execution for FSC nodes...` without proceeding.
**Cause**: Often due to the Sidecar service not being reachable by the FSC nodes.
**Fix (Implemented)**: We previously faced an issue where the sidecar port was dynamic (e.g., 5420) but hardcoded to 5411 in the client. This is fixed in `integration/nwo/fabricx/extensions/scv2`, but ensure that:
1.  The container is running: `docker ps`
2.  The port mapping is correct: `docker port <container_id>`

### 2. Endorsement Mismatches
**Symptom**: "received different results" error during endorsement.
**Cause**: Different nodes seeing different namespace versions.
**Fix**: Ensure `platform/fabricx/core/transaction/transaction.go` uses the *received* RWSet bytes rather than re-serializing locally.

### 3. Debug Logging
To assist with debugging, you can re-enable detailed logging. By default, `views/` now use clean `INFO` level logging, but you can add `DEBUG` logs back if needed.

```go
var logger = logging.MustGetLogger()

// In your view:
logger.Infof("[ViewName] message: %v", value)
// logger.Debugf("[ViewName] detailed: %+v", object) // Uncomment for deep debug
```

### 4. Check Endorsement Results
Add to `endorsement.go` if you suspect mismatch issues:
```go
if !bytes.Equal(res, proposalResponse.Results()) {
    logger.Errorf("MISMATCH: expected len=%d, got len=%d", len(res), len(proposalResponse.Results()))
    logger.Errorf("Expected: %x", res[:min(64, len(res))])
    logger.Errorf("Got:      %x", proposalResponse.Results()[:min(64, len(proposalResponse.Results()))])
}
```

### 5. Check RWSet Contents
```go
rwset, _ := tx.GetRWSet()
for ns := range rwset.Namespaces() {
    logger.Debugf("Namespace: %s", ns)
    writes, _ := rwset.GetWriteAt(ns, 0)
    for _, w := range writes {
        logger.Debugf("  Write: key=%s, len=%d", w.Key, len(w.Value))
    }
}
```

### Docker Container Logs

```bash
# Find the container
docker ps

# View logs
docker logs <container_id> 2>&1 | tail -100

# Follow logs in real-time
docker logs -f <container_id>
```

### Kill Stuck Tests

```bash
# Find processes
pgrep -a "ginkgo|tokenx"

# Kill them
pkill -9 ginkgo
pkill -9 tokenx.test

# Clean Docker
docker stop $(docker ps -q)
docker rm $(docker ps -aq)
```

---

## Quick Reference

### Important Files

| Purpose | File |
|---------|------|
| Test entry | `integration/fabricx/tokenx/tokenx_test.go` |
| Issue logic | `integration/fabricx/tokenx/views/issue.go` |
| Approver logic | `integration/fabricx/tokenx/views/approver.go` |
| Data models | `integration/fabricx/tokenx/states/states.go` |
| **FIX LOCATION** | `platform/fabricx/core/transaction/transaction.go` |
| RWSet serialization | `platform/fabricx/core/vault/interceptor.go` |
| Endorsement collection | `platform/fabric/services/endorser/endorsement.go` |
| Finality service | `platform/fabricx/core/finality/nlm.go` |

### Key Imports

```go
import (
    // FSC Core
    "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
    
    // Fabric Services
    "github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
    "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
    
    // FabricX Specific
    "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
    
    // Logging
    "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)
```

### Test Commands

```bash
# Run with verbose output
go test -v -run=TestEndToEnd

# Run with Ginkgo
ginkgo -v

# Run via Makefile
make integration-tests-fabricx-tokenx
```
