# I O(we) (yo)U, Deployment

In this tutorial we will cover the build and deployment process of our [IOU sample](../iou).
While the [IOU sample](../iou) is focusing on the basic use of the Fabric Smart Client, we have disregarded the build and deployment of that application by using our [Integration test framework](../../integration/nwo), which 

We recommend completing with the [IOU samples](../iou) before continuing here.

We will cover the following topics:
- Setup a Fabric network
- Generate crypto material
- Building the FSC node
- Configuration
- Run the node

## Setup a Fabric network

To illustrate how to use the Fabric Smart Client with an existing Fabric network, we will use microFab (TBD?!?!?)) as our Fabric network with the following configuration.

1. Three organization: org1, org2, and org3;
2. Single channel;
3. org1 needs to endorse iou transactions.

To start the network run:
```bash
just microfab
```

```bash
just install-dummy-cc
```

TODO describe other options to start a fabric network, e.g., fabric-samples test-network, MiniFab, etc ...

### Build Dummy Chaincode

Even though the IOU sample does not use a chaincode, we need to install a dummy chaincode to create a new namespace on our channel.
We provide a NO-OP dummy chaincode in `integration/nwo/fabric/chaincode/base`

```bash
go build -o cc github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/chaincode/base

# or
go build -o cc ../../../integration/nwo/fabric/chaincode/base
```

### Create FSC crypto material

TODO add

- FSC node crypto material
  - p2p
  - grpc
  - tls

Alternative: One can also use Fabric client identities.

## FSC Node

The business logic of the IOU sample is implemented using the View API and is executed by so called FSC nodes. Every participant (i.e., borrower, lender, and the approver) in the IOU sample hosts a FSC node. 

### Build

We provide the code for the FSC nodes in [nodes](nodes) for the borrower, the lender, and the approver.
The main function binds the view implementations to the FSC node for each party.
For example, the borrower uses the `CreateIOUView` 

```go
import ("github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/views")

registry := viewregistry.GetRegistry(n)
if err := registry.RegisterFactory("create", &views.CreateIOUViewFactory{}); err != nil {
return err
}
		
```

### Configuration

Before we can run the FSC node, we need to configure it by providing a `core.yaml`.
We provide a sample configuration file in [nodes//core.yaml]().

The configuration contains three major sections:
```yaml
# this section contains the configuration related to the FSC node
fsc:
  id: fsc.borrower
  networkId: 7jxtlwbgxfachim4clepdnqu7q
  # grpc view manager endpoint
  address: 127.0.0.1:20010
  addressAutoDetect: true
  listenAddress: 127.0.0.1:20010 # TODO is this redundant with address?
  
  # FSC node identity for p2p comm
  identity:
    cert:
      file: $FSC_CRYPTO/peerOrganizations/fsc.example.com/peers/alice.fsc.example.com/msp/signcerts/alice.fsc.example.com-cert.pem
    key:
      file: $FSC_CRYPTO/peerOrganizations/fsc.example.com/peers/alice.fsc.example.com/msp/keystore/priv_sk
  admin: # what is that used for?
    certs:
      - /Users/bur/Developer/gocode/src/github.com/hyperledger-labs/fabric-token-sdk/samples/nft/testdata/fsc/crypto/peerOrganizations/fsc.example.com/users/Admin@fsc.example.com/msp/signcerts/Admin@fsc.example.com-cert.pem
  tls:
    # this is for grpc-tls connection
  web:
  tracing:
  metrics:
  endpoint:
    resolvers:
    # resolvers used for p2p authentication
    - name: issuer
      domain: fsc.example.com
      identity:
        id: issuer
        path: $FSC_CRYPTO/peerOrganizations/fsc.example.com/peers/issuer.fsc.example.com/msp/signcerts/issuer.fsc.example.com-cert.pem
      addresses:
        P2P: 127.0.0.1:20001
      aliases:

# this section contains the configuration related to the fabric driver 
fabric:
  default:
    mspConfigPath: $CRYTPO/peerOrganizations/org2.example.com/peers/alice.org2.example.com/msp
    msps:
      - id: idemix
        mspType: idemix
        mspID: IdemixOrgMSP
        cacheSize: 0
        path: $CRYTPO/peerOrganizations/org2.example.com/peers/alice.org2.example.com/extraids/idemix
    tls:
    peers:
    channel:
    vault:

```

### Run

```bash
cd nodes/borrower
go build -o borrower
./borrower node start
```

### Test

#### Via curl

```bash
curl \
  --cacert /Users/bur/Developer/gocode/src/github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/testdata/fsc/crypto/peerOrganizations/fsc.example.com/peers/approver.fsc.example.com/tls/ca.crt \
  --key /Users/bur/Developer/gocode/src/github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/testdata/fsc/crypto/peerOrganizations/fsc.example.com/peers/approver.fsc.example.com/tls/server.key \
  --cert /Users/bur/Developer/gocode/src/github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/testdata/fsc/crypto/peerOrganizations/fsc.example.com/peers/approver.fsc.example.com/tls/server.crt \
  https://localhost:20002/
```

#### Via Test client

For this exercise we provide a small helper [client](client/main.go) that can connect to a FSC node via the web-based View API and trigger a view invocation.

## Action Required: Complete the journey

Next, it's your turn to build and run the FSC nodes for the lender and the approver.
s