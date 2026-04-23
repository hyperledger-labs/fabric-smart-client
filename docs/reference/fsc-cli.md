# FSC CLI Reference

## Overview

The Fabric Smart Client CLI (`fsccli`) is a helper utility command-line tool for developing and testing FSC-based application. It allows for managing FSC nodes, generating artifacts, managing cryptographic material, and invoking views on running FSC nodes.

## Installation

```bash
# Build from source
cd fabric-smart-client
make fsccli

# The binary will be available at:
# ./bin/fsccli
```


## Usage

```bash
fsccli [command] [flags]
```

### Environment Variables

All flags can be set via environment variables using the `FSCCLI_` prefix:

```bash
# Example: Set config file via environment variable
export FSCCLI_CONFIG=/path/to/config.yaml
```

Environment variable names use underscores instead of dots:
- `fsccli.config.path` → `FSCCLI_CONFIG_PATH`



## Commands

### `version`

Display version information for the FSC CLI.

**Usage:**
```bash
fsccli version
```

**Output:**
```
fsccli:
 Go version: go1.25.0
 OS/Arch: darwin/arm64
```

**Description:**
- Shows the CLI program name
- Displays Go version used to build the binary
- Shows operating system and architecture


### `artifactsgen`

Generate network artifacts from topology files.

**Usage:**
```bash
fsccli artifactsgen gen [flags]
```

#### Subcommands

##### `gen`

Generate artifacts from a topology configuration file.

**Flags:**
- `-t, --topology <file>` - **(Required)** Path to topology file in YAML format
- `-o, --output <dir>` - Output directory for generated artifacts (default: `./out/testdata`)
- `-p, --port <number>` - Starting port number for host services (default: `20000`)

**Examples:**

1. **Generate artifacts with default output:**
```bash
fsccli artifactsgen gen \
  --topology ./topology.yaml
```

2. **Generate artifacts with custom output directory:**
```bash
fsccli artifactsgen gen \
  --topology ./network-topology.yaml \
  --output ./generated-artifacts \
  --port 30000
```

3. **Using environment variables:**
```bash
export FSCCLI_TOPOLOGY=./topology.yaml
export FSCCLI_OUTPUT=./artifacts
export FSCCLI_PORT=25000
fsccli artifactsgen gen
```

**Topology File Format:**

The topology file must be in YAML format and define network topologies:

```yaml
topologies:
  - type: fabric
    name: default
    # Fabric network configuration...
  
  - type: fsc
    name: fsc-network
    # FSC network configuration...
```
**For more topology examples, see the integration tests in [`integration/`](../../integration/) directory.**

**Supported Topology Types:**
- `fabric` - Hyperledger Fabric(x) network topology
- `fsc` - Fabric Smart Client network topology

**Output:**

Generated artifacts include:
- Cryptographic material (certificates, keys)
- Configuration files
- Connection profiles
- Network topology artifacts


### `cryptogen`

Generate cryptographic material for network participants.

**Usage:**
```bash
fsccli cryptogen [command] [flags]
```

#### Subcommands

##### `generate`

Generate cryptographic material from a configuration template.

**Flags:**
- `-c, --config <file>` - **(Required)** Path to configuration template file
- `-o, --output <dir>` - Output directory for crypto material (default: `crypto-config`)

**Examples:**

1. **Generate crypto material:**
```bash
fsccli cryptogen generate \
  --config ./crypto-config.yaml \
  --output ./crypto-material
```

2. **Using default output directory:**
```bash
fsccli cryptogen generate --config ./crypto-config.yaml
```

**Configuration File Format:**

The configuration file defines organizations and their cryptographic requirements:

```yaml
# Example crypto-config.yaml
OrdererOrgs:
  - Name: Orderer
    Domain: example.com
    Specs:
      - Hostname: orderer

PeerOrgs:
  - Name: Org1
    Domain: org1.example.com
    EnableNodeOUs: true
    Template:
      Count: 2
    Users:
      Count: 1
```

##### `showtemplate`

Display the default configuration template.

**Usage:**
```bash
fsccli cryptogen showtemplate
```

**Example:**
```bash
# View default template
fsccli cryptogen showtemplate

# Save to file
fsccli cryptogen showtemplate > my-crypto-config.yaml
```

**Output:**

Displays a complete example configuration template that can be customized for your network.


### `view`

Invoke a view on a running FSC node.

**Usage:**
```bash
fsccli view [flags]
```

#### Flags

**Required:**
- `-f, --function <name>` - Name of the view function to invoke
- `-e, --endpoint <host:port>` - FSC node endpoint (or use config file)

**Input Options:**
- `-i, --input <data>` - Input data (base64 encoded or plain text)
- `-s, --stdin` - Read input from standard input

**Configuration:**
- `-c, --configFile <file>` - Load configuration from file

**TLS Configuration:**
- `-a, --peerTLSCA <file>` - TLS CA certificate for peer verification
- `-t, --tlsCert <file>` - Client TLS certificate (optional, for mutual TLS)
- `-k, --tlsKey <file>` - Client TLS key (optional, for mutual TLS)

**Authentication:**
- `-r, --userCert <file>` - User certificate for message authentication
- `-u, --userKey <file>` - User key for message signing

#### Examples

**1. Invoke view with inline input:**
```bash
fsccli view \
  --endpoint localhost:9000 \
  --function "QueryAsset" \
  --input "asset123" \
  --peerTLSCA ./tls-ca.pem \
  --userCert ./user-cert.pem \
  --userKey ./user-key.pem
```

**2. Invoke view with base64 encoded input:**
```bash
# Encode input
INPUT=$(echo -n '{"id":"asset123"}' | base64)

fsccli view \
  --endpoint localhost:9000 \
  --function "QueryAsset" \
  --input "$INPUT" \
  --peerTLSCA ./tls-ca.pem \
  --userCert ./user-cert.pem \
  --userKey ./user-key.pem
```

**3. Invoke view with stdin input:**
```bash
echo '{"id":"asset123","owner":"Alice"}' | fsccli view \
  --endpoint localhost:9000 \
  --function "TransferAsset" \
  --stdin \
  --peerTLSCA ./tls-ca.pem \
  --userCert ./user-cert.pem \
  --userKey ./user-key.pem
```

**4. Using configuration file:**

> **Important:** Replace the example paths below with actual paths to your certificate and key files. The paths must point to existing files.

Create `view-config.yaml`:
```yaml
version: 1
address: localhost:9000
tlsconfig:
  peercacertpath: /absolute/path/to/tls-ca.pem
  certpath: /absolute/path/to/client-cert.pem      # Optional
  keypath: /absolute/path/to/client-key.pem        # Optional
  timeout: 10s
signerconfig:
  mspid: Org1MSP
  identitypath: /absolute/path/to/user-cert.pem
  keypath: /absolute/path/to/user-key.pem
```

Invoke view:
```bash
fsccli view \
  --configFile ./view-config.yaml \
  --function "QueryAsset" \
  --input "asset123"
```

**Where to find certificates:**
- Generated artifacts: `./out/testdata/fsc/nodes/<node-name>/crypto/`
- Integration test data: `./integration/<test-name>/testdata/`
- Your own PKI infrastructure

**Example with real paths from integration tests:**
```yaml
version: 1
address: localhost:9000
tlsconfig:
  peercacertpath: /Users/saed/go/src/github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/testdata/fsc/nodes/fsc.node1/tls-ca-cert.pem
signerconfig:
  identitypath: /Users/saed/go/src/github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/testdata/fsc/nodes/fsc.node1/iss/default-issuer-cert.pem
  keypath: /Users/saed/go/src/github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/testdata/fsc/nodes/fsc.node1/iss/default-issuer-priv-key.pem
```

**5. Invoke view without input:**
```bash
fsccli view \
  --endpoint localhost:9000 \
  --function "ListAllAssets" \
  --peerTLSCA ./tls-ca.pem \
  --userCert ./user-cert.pem \
  --userKey ./user-key.pem
```

#### Configuration File Format

```yaml
version: 1
address: <host:port>

tlsconfig:
  certpath: <path>           # Client TLS certificate (optional)
  keypath: <path>            # Client TLS key (optional)
  peercacertpath: <path>     # Peer TLS CA certificate (required)
  timeout: <duration>        # Connection timeout (e.g., 10s)

signerconfig:
  mspid: <string>            # MSP ID (optional)
  identitypath: <path>       # User certificate (required)
  keypath: <path>            # User private key (required)
```

#### Input Encoding

The `--input` flag accepts data in two formats:

1. **Plain text:** Passed as-is to the view
   ```bash
   --input "hello world"
   ```

2. **Base64 encoded:** Automatically decoded before passing to view
   ```bash
   --input "aGVsbG8gd29ybGQ="  # Decodes to "hello world"
   ```

The CLI automatically detects base64 encoding and decodes it.


### `hsm`

HSM (Hardware Security Module) related utilities.

> **Note:** This command is only available when FSC is built with PKCS#11 support (`-tags pkcs11`).

**Usage:**
```bash
fsccli hsm [command]
```

#### Subcommands

##### `show-slots`

Display available HSM slots and tokens.

**Usage:**
```bash
fsccli hsm show-slots
```

**Example:**
```bash
fsccli hsm show-slots
```

**Output:**
```
Tokens found:
ForFSC
TestToken
```

**Description:**

This command:
1. Automatically detects PKCS#11 library location
2. Reads HSM configuration from environment variables
3. Lists all available tokens in the HSM

**Environment Variables:**

The command uses these environment variables (if set):
- `PKCS11_LIB` - Path to PKCS#11 library
- `PKCS11_PIN` - HSM PIN
- `PKCS11_LABEL` - Token label

**Common HSM Libraries:**
- **SoftHSM:** `/usr/lib/softhsm/libsofthsm2.so`
- **Thales/Gemalto:** `/opt/nfast/toolkits/pkcs11/libcknfast.so`
- **AWS CloudHSM:** `/opt/cloudhsm/lib/libcloudhsm_pkcs11.so`


## Exit Codes

The FSC CLI uses standard exit codes:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error (invalid arguments, command failed, etc.) |
