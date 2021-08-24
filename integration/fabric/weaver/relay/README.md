# Data Transfer using Weaver Relay Service

In this Section, we will learn how to perform a data Transfer between two Fabric networks.
We have two business parties: 
- `Alice` is a client of the Fabric network `alpha`.
- `Bob` is a client of the Fabric network `beta`.
Alice and Bob engage in the following business process:
- Alice puts new data in `alpha` inside a given namespace of a given channel.
- Alice contacts Bob telling him new data is available in `alpha`.
- Bob receives the name of the variable set by Alice.
- Bob `queries` Fabric network `alpha`, using `Weaver`, to get a proof that what Alice said is trustable.
- If the previous step is successful, then Bob stores the same data in `beta` (inside a given namespace of a given channel).
- Bob acks Alice that he has completed his task.
- Finally, Alice checks that Bob has actually stored the data she put in `alpha`, using `Weaver`.

## Weaver

Weaver is a platform, a protocol suite, and a set of tools, to enable interoperation for data sharing and asset 
movements between independent networks built on heterogeneous blockchain, or more generally, distributed ledger, 
technologies, in a manner that preserves the core blockchain tenets of decentralization and security.