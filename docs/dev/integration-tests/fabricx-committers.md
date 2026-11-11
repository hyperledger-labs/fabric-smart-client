# Fabric-x committers in integration tests

The Fabric-x integration tests use the [fabric-x-committer-test-node](https://github.com/hyperledger/fabric-x-committer/pkgs/container/fabric-x-committer-test-node) container image. From it nwo starts:
- a mock ordering service in a container of its own, reached as `orderer.localhost`;
- a [committer](https://github.com/hyperledger/fabric-x-committer) for every organization with a peer, named `CommitterName(org)` (`Org1_committer_0`), each in a container of its own with its own Postgres.

Each FSC node uses its own organization's committers, or the first committer when its organization has none. The first organization's committer is the network's default, which nwo uses to deploy namespaces; startup waits until every committer lists them.

The containers resolve `orderer.localhost` through Docker's DNS, and the FSC nodes on the host resolve it to loopback through the system resolver. On Linux that needs systemd-resolved or nss-myhostname, as on most desktop distributions and GitHub runners.

## Committer container configuration

The committers are configured through the `nwofabricx.Topology` returned by `NewTopology()`,
`NewDefaultTopology()`, or `NewTopologyWithName()`. Every option has a default, so a standard
test needs none of them.

### Settings for every committer

| Method | Default | Description |
|---|---|---|
| `WithCommitterImage(image string)` | `ghcr.io/hyperledger/fabric-x-committer-test-node:1.0.7` (`scv2.DefaultCommitterImage`) | Docker image for every committer container and the orderer container. |
| `WithCommitterEnv(key, value string)` | *(see below)* | Set an environment variable of every committer container; an empty value removes a default. |
| `WithOrdererEnv(key, value string)` | *(see below)* | Set an environment variable of the mock orderer container, such as `SC_ORDERER_BLOCK_SIZE`; an empty value removes a default. |

### Adding committers

`AddCommitter(org string, opts ...CommitterOption)` adds a committer to `org`, such as a second one
in an organization:

| Option | Default | Description |
|---|---|---|
| `WithName(name string)` | next free `<org>_committer_<n>` | Peer name for the committer's sidecar identity in the Fabric network. |
| `WithHost(host string)` | *(unset)* | Extra host name in the committer's TLS certificate; FSC nodes reach every committer at `127.0.0.1`. |
| `WithPorts(ports api.Ports)` | *(auto)* | Pre-allocated ports; leave nil to let the test framework allocate them. |
| `WithImage(image string)` | the topology's | Docker image for this committer's container. |
| `WithEnv(key, value string)` | the topology's | Override or add an environment variable of this committer's container. |

Default environment variables set by the test framework; `SC_ORDERER_*` is read by the orderer container, the rest by the committer containers:

| Variable | Default value |
|---|---|
| `SC_SIDECAR_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_QUERY_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_COORDINATOR_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_ORDERER_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_VC_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_VERIFIER_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_ORDERER_BLOCK_SIZE` | `1` |
| `SC_SIDECAR_ORDERER_SIGNED_ENVELOPES` | `true` |
| `SC_SIDECAR_SERVER_MAX_CONCURRENT_STREAMS` | `0` |
| `SC_QUERY_SERVER_MAX_CONCURRENT_STREAMS` | `0` |
| Endpoints and MSP dirs | *(derived from network)* |

### Example

```go
fxTopo := nwofabricx.NewDefaultTopology()
fxTopo.AddOrganizationsByName("Org1", "Org2") // runs Org1_committer_0 and Org2_committer_0

// run a committer image built locally from a fabric-x-committer checkout
fxTopo.WithCommitterImage("hyperledger/fabric-x-committer-test-node:local")

// increase the orderer's block size and turn on its debug logging
fxTopo.
    WithOrdererEnv("SC_ORDERER_BLOCK_SIZE", "10").
    WithOrdererEnv("SC_ORDERER_LOGGING_LOGSPEC", "debug")

// remove a default committer env var entirely, on every committer
fxTopo.WithCommitterEnv("SC_VC_LOGGING_LOGSPEC", "")

// add a second committer to Org1, Org1_committer_1, and a third one named and logging at debug
fxTopo.AddCommitter("Org1")
fxTopo.AddCommitter("Org1", nwofabricx.WithName("audit"), nwofabricx.WithEnv("SC_SIDECAR_LOGGING_LOGSPEC", "debug"))
```

## Example: a simple Fabric-x network topology

The example below defines a simple Fabric-x network with two organizations (`Org1` and `Org2`) and two FSC application nodes (`alice` and `bob`).

In this setup:
- `alice` acts as the endorser for the `iou` namespace.
- The namespace creation policy is controlled by Org1.

```go
import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
)

func Topology() []api.Topology {
	fxTopo := nwofabricx.NewDefaultTopology()
	fxTopo.AddOrganizationsByName("Org1", "Org2")
	// set the namespace creation policy to Org1
	fxTopo.SetNamespaceApproverOrgs("Org1")
	// define a namespace named "iou" with Org1 as endorser
	fxTopo.AddNamespace("iou", topology.Unanimity("Org1"))

	fscTopo := fsc.NewTopology()
	fscTopo.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1")) // Org1 endorses the iou namespace
	// register alice's views with RegisterResponder and RegisterViewFactory ...
	fscTopo.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2"))
	// register bob's views ...

	// register the Fabric-x SDK on every node
	fscTopo.AddSDK(&fabricx.SDK{})

	return []api.Topology{fxTopo, fscTopo}
}
```

More examples are in [`integration/fabricx/`](../../../integration/fabricx/). How an application node uses several committers at runtime is described in [Several committers](../../platform/fabric-x/README.md#several-committers).
