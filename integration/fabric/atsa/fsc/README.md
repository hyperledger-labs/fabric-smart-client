# Secured Asset Transfer in Fabric with FSC

[`Here`](../chaincode/README.md) we learned about: 
1. The [Secured asset transfer in Fabric](`https://hyperledger-fabric.readthedocs.io/en/latest/secured_asset_transfer/secured_private_asset_transfer_tutorial.html`), 
a Fabric sample that demonstrates how an asset can be represented and traded between organizations in a Hyperledger Fabric blockchain channel, and its design.
2. How to use the Integration Test Infrastructure that comes with the Fabric Smart Client to programmatically 
test distributed applications written with Fabric.

That design shows how complex is to map the business logic to Fabric concepts 
like `state-based endorsement policy`, `implicit collections`, and so on. 
Indeed, here are some important points of friction we have encountered:
- `Ownership`: There is no obvious way to hide the owner of an asset. 
  The implicit collection's endorsement policy defines `ownership`, kind of abuse.
- `Leakage`:  The implicit collection name leaks the organization name the collection belongs to.
- `Thread Model - No rouge peer resistance`: Suppose there is a rogue peer in OrgA, 
  this peer can endorse a transfer of asset `A` to OrgB without setting the private information 
  into the OrgB's implicit collection. At validation time, a signature from OrgB will not be required. 
  In other words, no fair exchange can happen without a trusted third-party.

Therefore, in the next sections, we will answer the following question:
```
Can the Fabric Smart Client helps the developer to solve the above issues and 
write a distributed application that is closer to the business logic?
```
The quick answer is yes.

## Secured Asset Transfer in Fabric with FSC

In developing the secured asset transfer application with FSC, 
we will keep the same business logic for the sake of simplicity. 
Namely, there will be an issue phase, an agreement phase, and a transfer phase. 
However, we will change paradigm: The `asset_transfer` chaincode will not mediate anymore the interactions 
between business parties (issuers and asset owners). 
The business parties will interact directly to achieve the business goals.

We have already learned, when exploring the Fabric's hidden gems, that an endorser is just a network node 
that executes `some code` and possesses a signing key compatible with an endorsement policy. 
The node produces a signature to signal its approval. Therefore, if we equip an FSC node with 
a signing secret key accepted by our  endorsement policy, that FSC node can endorse Fabric transactions.

In more details, FSC will allow us to shift to the following paradigm: 
The business parties, issuers and asset owners, will prepare directly a Fabric transaction 
(RWSet included). Before submitting this transaction to the Fabric's ordering service, 
the transaction will be sent to a set of approvers (for a PoC, one is enough). 
The role of the approver is to check that the transaction is well-formed following certain 
rules and `endorse`, meaning signing, the transaction. 
The approvers have Fabric signing keys accepted by the `asset_transfer` chaincode's endorsement policy.

This shift of paradigm gives us the following benefits:
1. Business parties are central to the business processes and interactive directly to assemble transactions.
2. Their interactions remain private to them.
3. Each business party store temporary states, in its vault, representing the current progress in 
   a given business process.
4. Approvers, business parties whose role is to enforce `validity rules`, approve (or endorse) transactions.  
   (This is still necessary because Fabric supports only endorsement policies, therefore any validity check must 
   happen before).

All the above will become more clear in the next Sections.

## Design

### Business Objects

Here is the definition of an asset.
```
type Asset struct {   
  ObjectType         string        `json:"objectType"`   
  ID                 string        `json:"assetID"`   
  Owner              view.Identity `json:"owner"`   
  PublicDescription  string        `json:"publicDescription"`   
  PrivateProperties  []byte        `state:"hash" json:"privateProperties"`
}

func (a *Asset) GetLinearID() (string, error) {   
  return rwset.CreateCompositeKey("asset", []string{a.ID})
}
```
As the reader can see:
- Ownership is modelled using FSC identities directly;
- The Asset state carries public and private information together;
- Tagging allows the developer to choose which fields must appears on the ledger obfuscated.
- An asset has a `linear id` that uniquely identify the asset.

Regarding the agreement, we have two states. One represents the agreement to sell:
```
type AgreementToSell struct {   
  TradeID string            `json:"trade_id"`   
  ID      string            `json:"asset_id"`   
  Price   int                  `json:"price"`   
  Owner   view.Identity `json:"owner"`
}

func (a *AgreementToSell) GetLinearID() (string, error) {   
  return rwset.CreateCompositeKey("AgreementToSell", []string{a.ID})
}
```
The other represents the agreement to buy:
```
type AgreementToBuy struct {   
  TradeID  string            `json:"trade_id"`   
  ID       string            `json:"asset_id"`   
  Price    int                  `json:"price"`   
  Owner    view.Identity `json:"owner"`
}

func (a *AgreementToBuy) GetLinearID() (string, error) {   
  return rwset.CreateCompositeKey("AgreementToBuy", []string{a.ID})
}
```

In both cases, we have that:
- Ownership is modelled using FSC identities directly;
- The states have exactly the same fields though different linear IDs;
- The states will appear on the ledger obfuscated meaning that the state will be reflected in the RWS as a key-value pair whose `key` is the hash of the linear ID and `value` is the hash of the json representation of the state.

### Network Topology

We will now deal with two network topologies. One for Fabric and one for the FSC nodes.

For Fabric, we have:
- Distinct organizations for Issuers, Asset Owners, and Approvers;
- Single channel;
- A namespace “asset_transfer” that can be endorsed by the Approvers. Meaning that, in order to modify that namespace, the approvers must `endorse` the transaction.
- No support for SBE and Implicit collections required.

Accompanying the Fabric network, we have an FSC network with the following topology:
- FSC Nodes for `issuers`, `asset owners` and `approvers`. We will assume a single issuer and a single approver, just for simplicity.
- Each FSC Node:
    - Is equipped with a Fabric Identity belonging to the proper Org.
    - Connects to a Fabric Peer belonging to the proper Org.
    - Runs its own views representing its role in the business processes.

### Business Processes or Interactive Protocols

To Be Continued...
