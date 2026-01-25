# TokenX - FabricX Token Management Application

A comprehensive token management system built on FabricX with privacy-preserving identities, multiple token types, and atomic swaps.

## Features

- **Four Roles (current topology)**: Issuer, Approver, Owner, Auditor
- **UTXO Token Model**: Tokens as discrete states with splitting support
- **Multiple Token Types**: USD, EUR, GOLD, or any custom type
- **Transfer Limits**: Configurable per-transaction limits
- **Atomic Swaps**: Swap views are registered on owner nodes; fully verified by integration tests
- **REST API**: OpenAPI + handlers under `api/`, wired into the integration topology and served by the FSC web server
- **Decimal Support**: 8 decimal places precision

## Quick Start

### Prerequisites

- Go 1.19+
- Docker (for Fabric network)
- Make

### Run Tests

```bash
cd /path/to/fabric-smart-client/integration/fabricx/tokenx

# Run all tests
go test -v ./...

# Or with Ginkgo
ginkgo -v
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        TokenX Network                           │
├─────────────────────────────────────────────────────────────────┤
│  Fabric Topology                                                │
│  - 1 Organization: Org1                                         │
│  - Namespace: tokenx with Org1 endorsement (unanimity)           │
├─────────────────────────────────────────────────────────────────┤
│  FSC Nodes                                                      │
│  ┌──────────┐  ┌──────────┐  ┌────────────────────────┐        │
│  │  Issuer  │  │ Approver │  │  Owners                 │        │
│  │  (Org1)  │  │  (Org1)  │  │  alice, bob, charlie    │        │
│  │  - issue │  │ - endorse│  │  - transfer             │        │
│  │  - init  │  │          │  │  - redeem               │        │
│  └──────────┘  └──────────┘  │  - query                │        │
│                              └────────────────────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

## Token Operations

### Issue Tokens

The issuer creates new tokens and assigns them to an owner:

```go
// Issue 1000 USD tokens to Alice
result, _ := client.CallView("issue", &views.Issue{
    TokenType: "USD",
    Amount:    states.TokenFromFloat(1000), // 100000000000
    Recipient: aliceIdentity,
})
```

### Transfer Tokens

Owners can transfer tokens to other owners:

```go
// Alice transfers 300 USD to Bob
result, _ := client.CallView("transfer", &views.Transfer{
    TokenLinearID: "<token_id>",
    Amount:        states.TokenFromFloat(300),
    Recipient:     bobIdentity,
    Approver:      issuerIdentity,
})
```

**Partial transfers** are supported - if you transfer less than the token amount, a "change" token is created for the sender.

### Redeem Tokens

Owners can burn tokens (with issuer approval):

```go
result, _ := client.CallView("redeem", &views.Redeem{
    TokenLinearID: "<token_id>",
    Amount:        states.TokenFromFloat(100),
    Approver:      issuerIdentity,
})
```

### Atomic Swap

Exchange tokens of different types atomically:

```go
// Swap views are registered on owner nodes in the integration topology.
// Alice proposes: give 100 USD, want 80 EUR
proposalID, _ := aliceClient.CallView("swap_propose", &views.SwapPropose{
    OfferedTokenID:  "<token_id>",
    RequestedType:   "EUR",
    RequestedAmount: states.TokenFromFloat(80),
    ExpiryMinutes:   60,
})

// Bob accepts with his EUR token
txID, _ := bobClient.CallView("swap_accept", &views.SwapAccept{
    ProposalID:     proposalID,
    OfferedTokenID: "<token_id>",
    Approver:       issuerIdentity,
})
```

## Token Amounts

All amounts use **8 decimal places** precision (similar to Bitcoin satoshis):

| Display Amount | Internal Value |
|----------------|----------------|
| 1.00000000 | 100000000 |
| 0.50000000 | 50000000 |
| 0.00000001 | 1 |

Use the helper functions:
```go
amount := states.TokenFromFloat(100.5)  // 10050000000
display := token.AmountFloat()          // 100.5
```

## Transfer Limits

Default transfer limits are configured in `states/states.go`:

| Limit | Default Value |
|-------|---------------|
| Max per transaction | 1,000,000 tokens |
| Min amount | 0.00000001 tokens |
| Daily limit | Unlimited |

## REST API

The API is documented in OpenAPI 3.0 format: `api/openapi.yaml`.

TokenX REST routes are mounted under the FSC node web server (base path: `/v1`). In the integration topology, the web server is enabled and uses HTTPS (self-signed certs).

- By default, the web server does **not** require a client certificate (`fsc.web.tls.clientAuthRequired: false`).
- If you enable mutual TLS, you must present a client certificate trusted by the node.

To find the concrete URL/port for a node, look at its generated node config (`fsc.web.address`) in the integration run directory.

Example (local dev, ignoring self-signed cert verification):

```bash
curl -k https://127.0.0.1:<web-port>/v1/tokens/balance
```

Note: The API is fully functional and integrated with the sidecar QueryService.

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/tokens/issue` | Issue new tokens |
| POST | `/v1/tokens/transfer` | Transfer tokens |
| POST | `/v1/tokens/redeem` | Redeem/burn tokens |
| GET | `/v1/tokens/balance` | Get token balance |
| GET | `/v1/tokens/history` | Get transaction history |
| POST | `/v1/tokens/swap/propose` | Propose atomic swap |
| POST | `/v1/tokens/swap/accept` | Accept atomic swap |
| GET | `/v1/audit/balances` | Auditor: all balances |
| GET | `/v1/audit/history` | Auditor: all transactions |

## Project Structure

```
tokenx/
├── api/
│   ├── handlers.go         # REST API handlers
│   └── openapi.yaml         # API documentation
├── states/
│   └── states.go            # Token, TransactionRecord, SwapProposal
├── views/
│   ├── issue.go             # Issue tokens
│   ├── transfer.go          # Transfer tokens
│   ├── redeem.go            # Burn tokens
│   ├── balance.go           # Query balances
│   ├── auditor.go           # Auditor views
│   ├── swap.go              # Atomic swaps
│   ├── approver.go          # Validation logic
│   └── utils.go             # Helpers
├── sdk.go                   # SDK registration
├── topology.go              # Network topology
├── tokenx_test.go           # Integration tests
├── tokenx_suite_test.go     # Test suite
└── README.md                # This file
```

## Identities and privacy

The current integration topology uses standard Fabric identities (Org1) for all nodes. The code uses `view.Identity` throughout, so it can be extended to support anonymous identities, but Idemix is not enabled/configured in the current TokenX integration topology.

## Development

### Adding a New Token Type

Simply issue tokens with a new type name:
```go
IssueTokens(ii, "MY_NEW_TOKEN", amount, "alice")
```

### Extending Swap Functionality

The swap implementation is designed for extension. Key areas:
- `SwapProposal` struct in `states/states.go` - add new fields
- `validateSwap` in `views/approver.go` - add new validations
- Add new swap-related views as needed

## Development Notes

### Running Integration Tests

**Important:** Always clean Docker before running tests:

```bash
# Clean up Docker environment
docker stop $(docker ps -q) 2>/dev/null
docker rm $(docker ps -aq) 2>/dev/null
docker network prune -f

# Verify port 7050 is free
sudo lsof -i :7050 || echo "Port 7050 is free"

# Run the test
cd /path/to/fabric-smart-client
make integration-tests-fabricx-tokenx
```

### Known Issues & Fixes

#### RWSet Endorsement Mismatch (FIXED)

**Issue:** Endorsement collection failed with "received different results" error.

**Root Cause:** FabricX's RWSet serialization includes namespace versions read from the local vault. When the approver re-serialized the RWSet, it used its own versions which differed from the issuer's.

**Fix Applied:** Modified `platform/fabricx/core/transaction/transaction.go` to use received RWSet bytes directly instead of re-serializing. See [TASK.md](TASK.md) for details.

#### Sidecar Port Mismatch (FIXED)

**Issue:** Test hanged at "Post execution for FSC nodes...".

**Root Cause:** The Sidecar container used a dynamic port (e.g., 5420), but the client configuration was hardcoded to `5411`.

**Fix Applied:** Updated `integration/nwo/fabricx/extensions/scv2` to dynamically propagate the correct sidecar port to the client configuration.

#### Docker Port Conflicts

**Issue:** Test fails with "port 7050 already allocated"

**Solution:** Clean Docker containers before running tests (see above).

### Comparing with Simple Example

The `integration/fabricx/simple/` project is a minimal working example of the same pattern. When debugging tokenx, compare with simple:

| TokenX | Simple |
|--------|--------|
| `views/issue.go` | `views/create.go` |
| `views/approver.go` | `views/approve.go` |
| `states/states.go` | `views/state.go` |
| `topology.go` | `topo.go` |

### Debug Logging

The codebase has extensive debug logging. Enable by checking `fsc.SetLogging()` in topology.go:

```go
fscTopology.SetLogging("grpc=error:fabricx=debug:info", "")
```

### Documentation

- [TASK.md](TASK.md) - Current development status and remaining work
- [WALKTHROUGH.md](WALKTHROUGH.md) - Detailed code walkthrough
- [SPECIFICATION.md](SPECIFICATION.md) - Full system specification

## License

Apache-2.0
