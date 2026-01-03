# TokenX - Token Management System Specification

## Table of Contents

1. [Overview](#overview)
2. [Requirements](#requirements)
3. [Architecture](#architecture)
4. [Data Models](#data-models)
5. [Network Topology](#network-topology)
6. [Operations & Flows](#operations--flows)
7. [REST API Specification](#rest-api-specification)
8. [Security & Privacy](#security--privacy)
9. [Configuration](#configuration)
10. [Testing](#testing)
11. [Development Guide](#development-guide)

---

## Overview

TokenX is a **token management system** built on FabricX (Hyperledger Fabric Smart Client). It provides a complete solution for issuing, transferring, redeeming, and swapping fungible tokens with privacy-preserving features.

### Key Capabilities

| Capability | Description |
|------------|-------------|
| **Multi-Token Support** | Issue any token type (USD, EUR, GOLD, custom) |
| **UTXO Model** | Tokens as discrete states with splitting |
| **Privacy** | Idemix anonymous identities for owners |
| **Atomic Swaps** | Exchange different token types atomically |
| **Audit Trail** | Complete transaction history |
| **Transfer Limits** | Configurable per-transaction limits |
| **REST API** | Documented OpenAPI 3.0 specification |

### Roles

| Role | Organization | Capabilities |
|------|--------------|--------------|
| **Issuer** | Org1 | Issue tokens, approve transactions, view issuance history |
| **Auditor** | Org2 | View all balances and history (no private data) |
| **Owner** | Org3 + Idemix | Hold, transfer, redeem tokens, propose/accept swaps |

---

## Requirements

### Functional Requirements

1. **FR-001**: Issuers can create tokens of any type with specified amounts
2. **FR-002**: Owners can transfer tokens to other owners
3. **FR-003**: Transfers support partial amounts (token splitting)
4. **FR-004**: Owners can redeem (burn) tokens with issuer approval
5. **FR-005**: Owners can view their token balances by type
6. **FR-006**: Owners can view their transaction history
7. **FR-007**: Auditors can view aggregate balances (not private data)
8. **FR-008**: Auditors can view all transaction history
9. **FR-009**: Issuers can view issuance and redemption history
10. **FR-010**: Owners can propose atomic swaps between token types
11. **FR-011**: Owners can accept swap proposals atomically
12. **FR-012**: All token amounts support 8 decimal places
13. **FR-013**: Transfer limits are enforced per transaction

### Non-Functional Requirements

1. **NFR-001**: Owner identities are privacy-preserving (Idemix)
2. **NFR-002**: All operations require appropriate endorsements
3. **NFR-003**: REST API follows OpenAPI 3.0 specification
4. **NFR-004**: System is extensible for future enhancements

---

## Architecture

### System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         TokenX System                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────────────┐ │
│  │   REST API   │────▶│  FSC Views   │────▶│   Fabric Network     │ │
│  │   (HTTP)     │     │  (Business   │     │   (State Storage)    │ │
│  │              │     │   Logic)     │     │                      │ │
│  └──────────────┘     └──────────────┘     └──────────────────────┘ │
│                                                                      │
├─────────────────────────────────────────────────────────────────────┤
│                         FSC Nodes                                    │
│  ┌────────────┐  ┌────────────┐  ┌────────────────────────────────┐ │
│  │   Issuer   │  │  Auditor   │  │         Owners (Idemix)        │ │
│  │   (Org1)   │  │   (Org2)   │  │  alice   │   bob   │  charlie  │ │
│  │            │  │            │  │  (Org3)  │  (Org3) │   (Org3)  │ │
│  └────────────┘  └────────────┘  └────────────────────────────────┘ │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                           tokenx/                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐                │
│  │   states/   │   │   views/    │   │    api/     │                │
│  │             │   │             │   │             │                │
│  │ • Token     │   │ • issue     │   │ • handlers  │                │
│  │ • TxRecord  │   │ • transfer  │   │ • openapi   │                │
│  │ • Swap      │   │ • redeem    │   │             │                │
│  │ • Limits    │   │ • balance   │   └─────────────┘                │
│  │             │   │ • auditor   │                                  │
│  └─────────────┘   │ • swap      │   ┌─────────────┐                │
│                    │ • approver  │   │  topology   │                │
│                    └─────────────┘   │     sdk     │                │
│                                      └─────────────┘                │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Data Models

### Token

The fundamental unit of value in the system.

```go
type Token struct {
    Type      string        // Token type (e.g., "USD", "EUR", "GOLD")
    Amount    uint64        // Amount in smallest units (8 decimals)
    Owner     view.Identity // Current owner (Idemix identity)
    LinearID  string        // Unique identifier (auto-generated)
    IssuerID  string        // Original issuer reference
    CreatedAt time.Time     // Creation timestamp
}
```

**Amount Encoding**:
- 8 decimal places precision
- `100000000` = 1.00000000 tokens
- `1` = 0.00000001 tokens

| Display | Internal Value |
|---------|----------------|
| 1.00 | 100000000 |
| 0.50 | 50000000 |
| 100.25 | 10025000000 |

### TransactionRecord

Audit trail for all token operations.

```go
type TransactionRecord struct {
    TxID           string        // Fabric transaction ID
    RecordID       string        // Unique record identifier
    Type           string        // "issue", "transfer", "redeem", "swap"
    TokenType      string        // Token type involved
    Amount         uint64        // Transaction amount
    From           view.Identity // Sender (empty for issue)
    To             view.Identity // Recipient (empty for redeem)
    Timestamp      time.Time     // When transaction occurred
    TokenLinearIDs []string      // IDs of tokens involved
}
```

### SwapProposal

Represents an offer to exchange tokens.

```go
type SwapProposal struct {
    ProposalID      string        // Unique proposal identifier
    OfferedTokenID  string        // LinearID of token being offered
    OfferedType     string        // Type of offered token
    OfferedAmount   uint64        // Amount being offered
    RequestedType   string        // Type wanted in return
    RequestedAmount uint64        // Amount wanted
    Proposer        view.Identity // Who proposed the swap
    Expiry          time.Time     // When proposal expires
    Status          string        // "pending", "accepted", "cancelled"
    CreatedAt       time.Time     // When proposal was created
}
```

### TransferLimit

Configurable limits for transfers.

```go
type TransferLimit struct {
    TokenType       string // Token type ("*" for all)
    MaxAmountPerTx  uint64 // Maximum per transaction
    MaxAmountPerDay uint64 // Maximum per day (0 = unlimited)
    MinAmount       uint64 // Minimum transfer amount
}
```

**Default Limits**:
| Limit | Default Value |
|-------|---------------|
| Max per transaction | 1,000,000 tokens |
| Max per day | Unlimited |
| Minimum | 0.00000001 tokens |

---

## Network Topology

### Fabric Network

```yaml
Fabric:
  Organizations:
    - Org1  # Issuer organization
    - Org2  # Auditor organization
    - Org3  # Owner organization
  
  Idemix:
    Enabled: true
    Organization: IdemixOrg
  
  Namespace:
    Name: tokenx
    Endorsement: Org1 (unanimity)
```

### FSC Nodes

#### Issuer Node (Org1)

```yaml
Node: issuer
Organization: Org1
Role: Approver
Views:
  - issue       # Issue new tokens
  - history     # View issuance/redemption history
  - init        # Initialize namespace processing
Responders:
  - ApproverView → IssueView
  - ApproverView → TransferView
  - ApproverView → RedeemView
  - ApproverView → SwapView
```

#### Auditor Node (Org2)

```yaml
Node: auditor
Organization: Org2
Views:
  - balances    # Query all token balances
  - history     # Query all transaction history
  - init        # Initialize auditor
Restrictions:
  - Cannot see token metadata
  - Cannot see private properties
  - Read-only access
```

#### Owner Nodes (Org3 + Idemix)

```yaml
Nodes: [alice, bob, charlie, ...]
Organization: Org3
Identity: Idemix (anonymous)
Views:
  - transfer      # Transfer tokens
  - redeem        # Burn tokens
  - balance       # Query own balance
  - history       # Query own history
  - swap_propose  # Create swap proposal
  - swap_accept   # Accept swap proposal
Responders:
  - AcceptTokenView → IssueView
  - TransferResponderView → TransferView
  - SwapResponderView → SwapView
```

---

## Operations & Flows

### Issue Tokens

**Participants**: Issuer → Recipient → Issuer (approver)

```
┌─────────┐          ┌───────────┐          ┌─────────┐
│ Issuer  │          │ Recipient │          │Approver │
│         │          │ (Owner)   │          │(Issuer) │
└────┬────┘          └─────┬─────┘          └────┬────┘
     │                     │                     │
     │ 1. Request identity │                     │
     │────────────────────▶│                     │
     │                     │                     │
     │ 2. Return Idemix ID │                     │
     │◀────────────────────│                     │
     │                     │                     │
     │ 3. Create token state                     │
     │ 4. Sign transaction │                     │
     │                     │                     │
     │ 5. Request endorsement                    │
     │────────────────────▶│                     │
     │                     │ 6. Validate & sign  │
     │◀────────────────────│                     │
     │                     │                     │
     │ 7. Request approval─────────────────────▶│
     │                     │                     │ 8. Validate:
     │                     │                     │    - 0 inputs
     │                     │                     │    - 1 token output
     │                     │                     │    - amount > 0
     │◀─────────────────────────────────────────│ 9. Sign
     │                     │                     │
     │ 10. Submit to orderer                     │
     │ 11. Wait for finality                     │
     ▼                     ▼                     ▼
```

**Validation Rules (Approver)**:
- No inputs (fresh token creation)
- Exactly 1 token output + 1 transaction record
- Positive token amount
- Valid token type (non-empty)
- Both issuer and recipient have signed

### Transfer Tokens

**Participants**: Sender → Recipient → Issuer (approver)

```
┌────────┐          ┌───────────┐          ┌─────────┐
│ Sender │          │ Recipient │          │Approver │
│(Owner) │          │  (Owner)  │          │(Issuer) │
└───┬────┘          └─────┬─────┘          └────┬────┘
    │                     │                     │
    │ 1. Request identity │                     │
    │────────────────────▶│                     │
    │                     │                     │
    │ 2. Return Idemix ID │                     │
    │◀────────────────────│                     │
    │                     │                     │
    │ 3. Load input token │                     │
    │ 4. Create output token(s)                 │
    │    (recipient token + change if partial)  │
    │ 5. Sign transaction │                     │
    │                     │                     │
    │ 6. Request endorsement                    │
    │────────────────────▶│                     │
    │                     │ 7. Validate & sign  │
    │◀────────────────────│                     │
    │                     │                     │
    │ 8. Request approval─────────────────────▶│
    │                     │                     │ 9. Validate:
    │                     │                     │    - input amount >= output
    │                     │                     │    - same token type
    │                     │                     │    - within limits
    │◀─────────────────────────────────────────│10. Sign
    │                     │                     │
    │ 11. Submit to orderer                     │
    ▼                     ▼                     ▼
```

**Partial Transfer Example**:
```
Input:  Token(USD, 1000, Alice)
Output: Token(USD, 300, Bob)     # Transferred
        Token(USD, 700, Alice)   # Change
```

**Validation Rules (Approver)**:
- At least 1 input, at least 2 outputs
- Output amount ≤ input amount
- Token types match
- Transfer limits respected
- Both sender and recipient signed

### Redeem Tokens

**Participants**: Owner → Issuer (approver)

```
┌────────┐                              ┌─────────┐
│ Owner  │                              │Approver │
│        │                              │(Issuer) │
└───┬────┘                              └────┬────┘
    │                                        │
    │ 1. Load token to redeem                │
    │ 2. Mark token for deletion             │
    │ 3. Create transaction record           │
    │ 4. Sign transaction                    │
    │                                        │
    │ 5. Request approval──────────────────▶│
    │                                        │ 6. Validate:
    │                                        │    - 1 input (token)
    │                                        │    - token deleted
    │                                        │    - owner signed
    │◀───────────────────────────────────────│ 7. Sign
    │                                        │
    │ 8. Submit to orderer                   │
    ▼                                        ▼
```

### Atomic Swap

**Two-phase protocol**:

#### Phase 1: Propose Swap

```
┌──────────┐
│ Proposer │
│ (Alice)  │
└────┬─────┘
     │
     │ 1. Load offered token
     │ 2. Verify ownership
     │ 3. Create SwapProposal:
     │    - OfferedTokenID
     │    - RequestedType
     │    - RequestedAmount
     │    - Expiry
     │ 4. Submit proposal
     ▼
  ProposalID
```

#### Phase 2: Accept Swap

```
┌──────────┐          ┌──────────┐          ┌─────────┐
│ Accepter │          │ Proposer │          │Approver │
│  (Bob)   │          │ (Alice)  │          │(Issuer) │
└────┬─────┘          └────┬─────┘          └────┬────┘
     │                     │                     │
     │ 1. Load proposal    │                     │
     │ 2. Verify not expired                     │
     │ 3. Load proposer's token                  │
     │ 4. Load own token   │                     │
     │ 5. Verify amounts match                   │
     │ 6. Create swapped tokens:                 │
     │    - Alice's token → Bob                  │
     │    - Bob's token → Alice                  │
     │ 7. Delete proposal  │                     │
     │ 8. Sign transaction │                     │
     │                     │                     │
     │ 9. Request endorsement                    │
     │────────────────────▶│                     │
     │                     │10. Validate & sign  │
     │◀────────────────────│                     │
     │                     │                     │
     │11. Request approval─────────────────────▶│
     │                     │                     │12. Validate
     │◀─────────────────────────────────────────│13. Sign
     │                     │                     │
     │14. Submit to orderer                      │
     ▼                     ▼                     ▼
```

**Swap Example**:
```
Before:
  Alice: Token(USD, 100)
  Bob:   Token(EUR, 80)

Swap Proposal (Alice):
  Offering: 100 USD
  Requesting: 80 EUR

After Acceptance:
  Alice: Token(EUR, 80)
  Bob:   Token(USD, 100)
```

---

## REST API Specification

### Base URL

```
http://localhost:8080/v1
```

### Endpoints

#### Issue Tokens

```http
POST /tokens/issue
Content-Type: application/json

{
  "token_type": "USD",
  "amount": "1000.00",
  "recipient": "alice"
}
```

**Response**:
```json
{
  "token_id": "TKN:abc123def456"
}
```

#### Transfer Tokens

```http
POST /tokens/transfer
Content-Type: application/json

{
  "token_id": "TKN:abc123def456",
  "amount": "300.00",
  "recipient": "bob"
}
```

**Response**:
```json
{
  "tx_id": "tx_789xyz"
}
```

#### Redeem Tokens

```http
POST /tokens/redeem
Content-Type: application/json

{
  "token_id": "TKN:abc123def456",
  "amount": "100.00"
}
```

**Response**:
```json
{
  "tx_id": "tx_redeem123"
}
```

#### Get Balance

```http
GET /tokens/balance?token_type=USD
```

**Response**:
```json
{
  "balances": {
    "USD": 1000.00,
    "EUR": 500.00
  },
  "tokens": [
    {
      "linear_id": "TKN:abc123",
      "type": "USD",
      "amount": 1000.00
    }
  ]
}
```

#### Get History

```http
GET /tokens/history?token_type=USD&tx_type=transfer&limit=100
```

**Response**:
```json
{
  "records": [
    {
      "tx_id": "tx_123",
      "type": "transfer",
      "token_type": "USD",
      "amount": 100.00,
      "from": "...",
      "to": "...",
      "timestamp": "2026-01-03T10:30:00Z"
    }
  ]
}
```

#### Propose Swap

```http
POST /tokens/swap/propose
Content-Type: application/json

{
  "offered_token_id": "TKN:usd123",
  "requested_type": "EUR",
  "requested_amount": 80.00,
  "expiry_minutes": 60
}
```

**Response**:
```json
{
  "proposal_id": "SWP:prop789"
}
```

#### Accept Swap

```http
POST /tokens/swap/accept
Content-Type: application/json

{
  "proposal_id": "SWP:prop789",
  "offered_token_id": "TKN:eur456"
}
```

**Response**:
```json
{
  "tx_id": "tx_swap123"
}
```

#### Auditor Endpoints

```http
GET /audit/balances?token_type=USD

GET /audit/history?token_type=USD&tx_type=issue&limit=1000
```

---

## Security & Privacy

### Identity Management

| Role | Identity Type | Privacy Level |
|------|---------------|---------------|
| Issuer | X.509 | Identified |
| Auditor | X.509 | Identified |
| Owner | Idemix | Anonymous |

### Idemix Features

- **Unlinkability**: Transactions from same owner cannot be linked
- **Selective Disclosure**: Owners can prove attributes without revealing identity
- **Revocation**: Compromised credentials can be revoked

### Endorsement Policy

```
Namespace: tokenx
Endorsement: Org1 must approve all transactions
```

### Authorization Matrix

| Operation | Issuer | Auditor | Owner |
|-----------|--------|---------|-------|
| Issue tokens | ✅ | ❌ | ❌ |
| Transfer tokens | ❌ | ❌ | ✅ |
| Redeem tokens | ❌ | ❌ | ✅ |
| View own balance | ✅ | ❌ | ✅ |
| View all balances | ❌ | ✅ | ❌ |
| View own history | ✅ | ❌ | ✅ |
| View all history | ❌ | ✅ | ❌ |
| Propose swap | ❌ | ❌ | ✅ |
| Accept swap | ❌ | ❌ | ✅ |
| Approve transactions | ✅ | ❌ | ❌ |

---

## Configuration

### Transfer Limits

Edit `states/states.go`:

```go
func DefaultTransferLimit() *TransferLimit {
    return &TransferLimit{
        TokenType:       "*",               // Apply to all types
        MaxAmountPerTx:  TokenFromFloat(1000000), // 1M per tx
        MaxAmountPerDay: 0,                 // Unlimited daily
        MinAmount:       1,                 // Minimum 0.00000001
    }
}
```

### Logging

In `topology.go`:

```go
fscTopology.SetLogging("grpc=error:fabricx=info:tokenx=debug:info", "")
```

### Swap Expiry

Default: 60 minutes. Configure per proposal:

```go
SwapPropose{
    ExpiryMinutes: 120, // 2 hours
}
```

---

## Testing

### Unit Tests

```bash
go test ./integration/fabricx/tokenx/states/...
```

### Integration Tests

```bash
# Run all tests
go test -v ./integration/fabricx/tokenx/...

# With Ginkgo
cd integration/fabricx/tokenx
ginkgo -v
```

### Test Scenarios

| Test | Description |
|------|-------------|
| Issue Flow | Issue tokens to owner, verify balance |
| Transfer Flow | Transfer between owners, verify balances |
| Partial Transfer | Transfer less than token amount, verify change |
| Redeem Flow | Burn tokens, verify removed |
| Multi-Token | Issue/transfer different token types |
| Swap Flow | Propose and accept atomic swap |
| Transfer Limits | Verify limits are enforced |
| Invalid Operations | Verify rejection of invalid transactions |

---

## Development Guide

### Directory Structure

```
integration/fabricx/tokenx/
├── api/
│   ├── handlers.go      # REST API handlers
│   └── openapi.yaml     # API documentation
├── states/
│   └── states.go        # Data models
├── views/
│   ├── issue.go         # Issue tokens
│   ├── transfer.go      # Transfer tokens
│   ├── redeem.go        # Burn tokens
│   ├── balance.go       # Query balances
│   ├── auditor.go       # Auditor views
│   ├── swap.go          # Atomic swaps
│   ├── approver.go      # Validation logic
│   └── utils.go         # Helpers
├── sdk.go               # SDK registration
├── topology.go          # Network topology
├── tokenx_test.go       # Integration tests
├── tokenx_suite_test.go # Test suite
├── Makefile             # Dev commands
└── README.md            # Documentation
```

### Adding a New Token Operation

1. **Define parameters** in a new view file:
   ```go
   type MyOperation struct {
       // Parameters
   }
   ```

2. **Implement the view**:
   ```go
   type MyOperationView struct {
       MyOperation
   }
   
   func (v *MyOperationView) Call(ctx view.Context) (interface{}, error) {
       // Implementation
   }
   ```

3. **Add validation** in `approver.go`:
   ```go
   case "my_operation":
       return a.validateMyOperation(ctx, tx)
   ```

4. **Register in topology**:
   ```go
   RegisterViewFactory("my_operation", &MyOperationViewFactory{})
   ```

5. **Add REST endpoint** in `handlers.go`

6. **Write tests** in `tokenx_test.go`

### Building

```bash
cd /path/to/fabric-smart-client

# Build
go build ./integration/fabricx/tokenx/...

# Verify
go vet ./integration/fabricx/tokenx/...

# Test
go test -v ./integration/fabricx/tokenx/...
```

---

## Appendix

### State Key Prefixes

| Prefix | State Type |
|--------|------------|
| `TKN` | Token |
| `TXR` | TransactionRecord |
| `SWP` | SwapProposal |

### Transaction Types

| Type | Command | Description |
|------|---------|-------------|
| Issue | `issue` | Create new tokens |
| Transfer | `transfer` | Move tokens |
| Redeem | `redeem` | Burn tokens |
| Swap Propose | `swap_propose` | Create swap offer |
| Swap | `swap` | Execute atomic swap |

### Amount Conversion Functions

```go
// Float to internal
amount := states.TokenFromFloat(100.5) // 10050000000

// Internal to float
display := token.AmountFloat() // 100.5

// Precision constant
states.DecimalPrecision // 8
states.DecimalFactor    // 100000000
```

---

## Technical Deep Dive: Endorsement Process

This section documents critical learnings from debugging the endorsement process.

### How FabricX Endorsement Works

```
┌──────────────────────────────────────────────────────────────────────────┐
│ Issuer Node                           │ Approver Node                     │
├───────────────────────────────────────┼───────────────────────────────────┤
│ 1. Create transaction                 │                                   │
│ 2. Add outputs to RWSet               │                                   │
│ 3. Serialize RWSet (with namespace    │                                   │
│    versions from local vault)         │                                   │
│ 4. Send transaction to approver ──────┼───────────────────────────────►   │
│                                       │ 5. Receive transaction            │
│                                       │ 6. Validate business logic        │
│                                       │ 7. Create ProposalResponse        │
│                                       │    (MUST use issuer's RWSet bytes)│
│ 8. Receive ProposalResponse ◄─────────┼───────────────────────────────    │
│ 9. Compare: issuer.Results() vs       │                                   │
│    proposalResponse.Results()         │                                   │
│ 10. If match: endorsement valid       │                                   │
└───────────────────────────────────────┴───────────────────────────────────┘
```

### Critical Implementation Detail

**File:** `platform/fabricx/core/transaction/transaction.go`

```go
func (t *Transaction) getProposalResponse(signer SerializableSigner) (*pb.ProposalResponse, error) {
    var rawTx []byte
    
    // KEY: Use received RWSet bytes if available (approver case)
    // This ensures endorsement results match the issuer's
    if len(t.RWSet) != 0 {
        rawTx = t.RWSet  // Use issuer's original serialization
    } else {
        // Issuer case: serialize from scratch
        rwset, err := t.GetRWSet()
        rawTx, _ = rwset.Bytes()
    }
    
    // ... create ProposalResponse with rawTx
}
```

### Why Namespace Versions Cause Issues

FabricX uses counter-based versioning stored in a `_meta` namespace. Each node maintains its own view of these versions:

```go
// In vault/interceptor.go
func (rws *Interceptor) namespaceVersions() map[string]uint64 {
    versions := make(map[string]uint64)
    
    // Reads from LOCAL vault - different nodes have different values!
    rwsIt, _ := rws.wrapped.NamespaceReads("_meta")
    for rwsIt.HasNext() {
        rw, _ := rwsIt.Next()
        versions[rw.Namespace] = rw.Block
    }
    
    return versions
}
```

**Problem:** If the approver calls `rwset.Bytes()`, it serializes with ITS versions, not the issuer's. This causes the byte comparison to fail.

**Solution:** Pass the issuer's serialized bytes through unchanged.

### Comparison with Simple Pattern

The `integration/fabricx/simple/` follows the exact same pattern:

| Component | Simple | TokenX |
|-----------|--------|--------|
| State creation | `views/create.go` | `views/issue.go` |
| Approval | `views/approve.go` | `views/approver.go` |
| Finality listener | `views/utils/listener.go` | `views/utils.go` |
| SDK | `sdk.go` | `sdk.go` |

Both use:
1. `state.NewTransaction(ctx)`
2. `state.NewCollectEndorsementsView(tx, approvers...)`
3. `finality.GetListenerManager()` + `AddFinalityListener()`
4. `state.NewOrderingAndFinalityWithTimeoutView(tx, timeout)`
5. `wg.Wait()` for finality

---

## Troubleshooting Reference

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| "received different results" | RWSet serialization mismatch | Ensure approver uses received RWSet bytes |
| "port 7050 already allocated" | Docker leftover | Clean Docker containers |
| "timeout waiting for finality" | Sidecar not processing | Check Docker logs, verify sidecar health |
| "interceptor already closed" | Double-close on vault | Check transaction lifecycle |

### Debug Commands

```bash
# Check Docker containers
docker ps -a

# View sidecar logs
docker logs $(docker ps -q --filter "ancestor=hyperledger/fabric-x-committer-test-node:0.1.5") 2>&1 | tail -50

# Check port usage
sudo lsof -i :7050

# Kill stuck processes
pkill -9 ginkgo
pkill -9 tokenx.test
```

### Useful Log Patterns

```bash
# Find endorsement issues
grep -i "received different" /path/to/log

# Find RWSet serialization
grep -i "rwset\|RWSet" /path/to/log

# Find finality events
grep -i "finality\|committed" /path/to/log
```

---

*Document Version: 1.1*  
*Last Updated: 2026-01-03*  
*Contributors: AI Assistant (Debugging & Documentation)*
