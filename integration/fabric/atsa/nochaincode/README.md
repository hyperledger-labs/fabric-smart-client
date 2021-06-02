# Secured Asset Transfer

There is nothing better than an example to start learning something new. One of the main goals of FSC is to speed up the development of Fabric-based distributed applications by offering a programming model that is closer to the business logic rather than to the blockchain technicalities.

## Secured Asset Transfer in Fabric

The [Secured asset transfer in Fabric](`https://hyperledger-fabric.readthedocs.io/en/latest/secured_asset_transfer/secured_private_asset_transfer_tutorial.html`) is a tutorial that demonstrates how an asset can be represented and traded between organizations in a Hyperledger Fabric blockchain channel.

### Business Objects

In this example, an asset is represented by the following data structure:
```
type Asset struct {   
  ObjectType             string
  ID                     string  
  OwnerOrg               string   
  PublicDescription      string
}
```
An asset:
- Is owned by a single organization (`OwnerOrg` field). State-Based Endorsement is use to model ownership;
- Comes equipped with some private information that is stored in the `implicit collection` of the owning organization;

An asset can be transferred if and only if an agreement between the current owner and the next owner is achieved.
An agreement to sell or buy is represented by the following data structure:
```
type Agreement struct {   
  ID      string   
  Price   int   
  TradeID string
}
```

### Network Topology and Business Operations

The distributed application is deployed on a vanilla Fabric network with:
- Multiple Organizations;
- Single channel;
- A Chaincode that coordinates the business operations and can be endorsed by any organization; The chaincode defines also a namespace into the ledger that stores the states.
- Support for State-Based Endorsement (SBE, for short) and Implicit collections.

The chaincode, installed on the peers of all organizations, handles the following business operations:
- `Issue`: Any organization can issue assets because the chaincode endorsement says that any organization can endorse. The issuer passes to the chaincode the asset private information via transient. The chaincode creates a new instance of the asset, filling all fields, and puts it in the RWS. Then, the chaincode sets the SBE policy to assign ownership to the calling organization. Finally, the chaincode stores the asset private information in the implicit collection of the issuer’s organization that is owning the asset.
- `Agree to sell and buy`: Before transferring an Asset, an agreement between the owning organization (`AgreeToSell`) and the recipient organization (`AgreeToBuy`) must be stored on the ledger. Organizations store their agreements in their implicit collections.  The endorsement policy of the implicit collections can be thought as the owner of the agreement.
  A `Hash` appears on the ledger for each agreement and this hash is available to the entire network. A transfer can happen only if there is a matching agreement to sell and buy.
  The chaincode performs access control and then stores the agreement on the implicit collection of the calling organization.
- `Transfer`: The asset owner passes to the chaincode the asset private information via transient. The chaincode first checks that the asset private information is compatible with the hash stored in owner’s organization implicit collection. If there is a match between the `AgreeToSell` and the `AgreeToBuy` (The match is performed by comparing the hashes. Notice that hashes are available to all peers in the network), the chaincode writes the private information in the next owner’s organization implicit collection (This requires an endorsement from the next owner’s organization). Finally, the chaincode changes the asset's owner to new receiving organization and stores the asset in the RWS.

### Issues with the above approach

The above description shows how complex is to model the business logic to Fabric concepts like state-based endorsement policy, implicit collections, and so on. Indeed, here are some important points of friction we can highlight:
- `Ownership`: There is no obvious way to hide the owner of an asset. The implicit collection endorsement policy is abused to define `ownership`.
- `Leakage`:  The implicit collection name leaks the organization name the collection belongs to.
- `Thread Model - No rouge peer resistance`: Suppose there is a rogue peer in Org1, this peer can endorse a transfer of asset `A` to Org2 without setting the private information into the Org2's implicit collection. At validation time, a signature from Org2 will not be required. In other words, no fair exchange can happen without a trusted third-party.

In the next sections, we will answer the following question:
```
Can the Smart Fabric Client helps the developer to solve the above issues and write a distributed application that is closer to the business logic?
```
The quick answer is yes.

## Secured Asset Transfer in Fabric with FSC

In developing the secured asset transfer application with FSC, we will keep the same business logic for the sake of simplicity. Namely, there will be an issue phase, an agreement phase, and a transfer phase. Though, we will change paradigm.
Namely, the chaincode will not mediate anymore the interactions between business parties (issuers and asset owners). The business parties will interact directly to achieve the business goals.

In the motivation section, we saw that an endorser is just a network node that executes `some code` and possesses a signing secret key, that is accepted by a given endorsement policy. The node produces a signature to signal its approval. Therefore, if we equip an FSC node with a signing secret key accepted by our reference endorsement policy, that FSC node can endorse Fabric transactions.

In more details, FSC will allow us to shift to the following paradigm: The business parties, issuers and asset owners, will prepare directly a Fabric transaction (RWSet included). Before submitting this transaction to the Fabric's ordering service, the transaction will be sent to a set of approvers (for a PoC, one is enough). The role of the approver is to check that the transaction is well formed following certain rules and `endorse`, meaning signing, the transaction. The approvers are equipped with the signing secret key accepted by our target endorsement policy.

This shift of paradigm gives us the following benefits:
1. Business parties are central to the business processes and interactive directly to assemble transactions.
2. Their interactions remain private to them.
3. Each business party store temporary states, in its vault, representing the current progress in a given business process.
4. Transactions are approved or endorsed by business parties whose role is to enforce `validity rules` (This is still necessary because Fabric supports only endorsement policies, therefore any validity check must happen before).

All of the above will become more clear in the next Sections.

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
