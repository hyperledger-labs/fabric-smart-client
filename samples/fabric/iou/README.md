# I O(we) (yo)U

In this section, we will cover a classic use-case, the `I O(we) (yo)U`.
Let us consider two parties: a `lender` and a `borrower`.
The borrower owes the lender a certain amount of money.
Both parties want to track the evolution of this amount.
Moreover, the parties want an `approver` to validate their operations.
We can think about the approver as a mediator between the parties.

Looking ahead, the approver plays the role of the endorser of the namespace
in a Fabric channel that contains the IOU state.
We have already learned, when exploring the ['Fabric's hidden gems'](./../../../docs/design.md#fabric-and-its-hidden-gems),
that an endorser is just a network node that executes `some code` and possesses a signing key compatible with an endorsement policy.
The node produces a signature to signal its approval. Therefore, if we equip an FSC node with
a signing secret key accepted by our endorsement policy, that FSC node can endorse Fabric transactions.

For this example, we will employ the `State-Based Programming Model` (SPM, for short) that the FSC introduces.
This model provides an API that helps the developer to think in terms of states rather than
RWSet. A Transaction in SPM wraps a Fabric Transaction and let the developer:
- Add `references` to input states. The equivalent of a read-dependency in the RWSet.
- `Delete` states. A write-entry in the RWSet that `deletes a key`.
- `Update` states. A write-entry in the RWSet that `updates a key's value`.
  With the above, it is easy to model a UTXO-based with reference states on top of a Fabric transaction.

It is time to deep dive. Let us begin by modelling the state the parties want to track:

## Business States

We just need to model a business state that represents the amount of money the borrower still owes the lender.
The state must be uniquely identifiable. Moreover, the state
should be owned or controllable by both the lender and the borrower. This means that any operation on the state
should be `agreed` between the lender and the borrower.

Here is a way to code the above description.

```go

// IOU models the IOU state
type IOU struct {
	// Amount the borrower owes the lender
	Amount   uint
	// Unique identifier of this state
	LinearID string
	// The list of owners of this state
	Parties  []view.Identity 
}

func (i *IOU) SetLinearID(id string) string {
	if len(i.LinearID) == 0 {
		i.LinearID = id
	}
	return i.LinearID
}

func (i *IOU) Owners() state.Identities {
	return i.Parties
}

```

Let us look more closely at the anatomy of this state:
- It has `three fields`:
    - The `Amount` the borrowers owes the lender;
    - A `unique identifier` to locate the state, and
    - The list of `owners` of the state.
- It has `two methods`:
    - `SetLinearID` that allows the platform to assign automatically a unique identifier to the state.
      Indeed, `IOU` implements the [`LinearState` interface](./../../../platform/fabric/services/state/state.go)
    - `Owners` that returns the identities of the owners of the state.
      Indeed, `IOU` implements the [`Ownable` interface](./../../../platform/fabric/services/state/state.go)

## Business Processes or Interactions

### Create the IOU State

The very first operation is to create the IOU state.
Let us assume that the borrower is the initiator of the interactive protocol to create this state.
This is the view the borrower executes:

```go

// Create contains the input to create an IOU state
type Create struct {
	// Amount the borrower owes the lender
	Amount uint
	// Lender is the identity of the lender's FSC node
	Lender view.Identity
	// Approver is the identity of the approver's FSC node
	Approver view.Identity
}

type CreateIOUView struct {
	Create
}

func (i *CreateIOUView) Call(context view.Context) (interface{}, error) {
    // use default identities if not specified
    if i.Lender.IsNone() {
        i.Lender = view2.GetIdentityProvider(context).Identity("lender")
    }
    if i.Approver.IsNone() {
        i.Approver = view2.GetIdentityProvider(context).Identity("approver")
    }

    // As a first step operation, the borrower contacts the lender's FSC node
	// to exchange the identities to use to assign ownership of the freshly created IOU state.
	borrower, lender, err := state.ExchangeRecipientIdentities(context, i.Lender)
	assert.NoError(err, "failed exchanging recipient identity")

	// The borrower creates a new transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err, "failed creating a new transaction")

	// Sets the namespace where the state should be stored
	tx.SetNamespace("iou")

	// Specifies the command this transaction wants to execute.
	// In particular, the borrower wants to create a new IOU state owned by the borrower and the lender
	// The approver will use this information to decide how validate the transaction
	assert.NoError(tx.AddCommand("create", borrower, lender))

	// The borrower prepares the IOU state
	iou := &states.IOU{
		Amount:  i.Amount,
		Parties: []view.Identity{borrower, lender},
	}
	// and add it to the transaction. At this stage, the ID gets set automatically.
	assert.NoError(tx.AddOutput(iou))

	// The borrower is ready to collect all the required signatures.
	// Namely from the borrower itself, the lender, and the approver. In this order.
	// All signatures are required.
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, borrower, lender, i.Approver))
	assert.NoError(err)

	// At this point the borrower can send the transaction to the ordering service and wait for finality.
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err)

	// Return the state ID
	return iou.LinearID, nil
}
```

The lender responds to request of endorsement from the borrower running the following view:

```go

type CreateIOUResponderView struct{}

func (i *CreateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// As a first step, the lender responds to the request to exchange recipient identities.
	lender, borrower, err := state.RespondExchangeRecipientIdentities(context)
	assert.NoError(err, "failed exchanging recipient identities")

	// When the leneder runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the lender. Therefore, the lender waits to receive the transaction.
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// The lender can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	assert.Equal(1, len(tx.Namespaces()), "expected only one namespace")
	assert.Equal("iou", tx.Namespaces()[0], "expected the [iou] namespace, got [%s]", tx.Namespaces()[0])

	// Commands are properly populated
	assert.Equal(1, tx.Commands().Count(), "expected only a single command, got [%s]", tx.Commands().Count())
	switch command := tx.Commands().At(0); command.Name {
	case "create":
		// If the create command is attached to the transaction then...
		assert.Equal(0, tx.NumInputs(), "invalid number of inputs, expected 0, was [%d]", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was [%d]", tx.NumInputs())

		iouState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, iouState))
		assert.False(iouState.Amount < 5, "invalid amount, expected at least 5, was [%d]", iouState.Amount)
		assert.Equal(2, iouState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", iouState.Owners().Count())
		assert.True(iouState.Owners().Contain(lender), "invalid state, it does not contain lender identity")

		assert.True(command.Ids.Match([]view.Identity{lender, borrower}), "the command does not contain the lender and borrower identities")
		assert.True(iouState.Owners().Match([]view.Identity{lender, borrower}), "the state does not contain the lender and borrower identities")
		assert.NoError(tx.HasBeenEndorsedBy(borrower), "the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, expected [create], was [%s]", command)
	}

	// The lender is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the lender waits the the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx))
}
```

On the other hand, the approver runs the following view to respond to the request of signature from the borrower:

```go

type ApproverView struct{}

func (i *ApproverView) Call(context view.Context) (interface{}, error) {
    // When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the approver. Therefore, the approver waits to receive the transaction.
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// The approver can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	assert.Equal(1, len(tx.Namespaces()), "expected only one namespace")
	assert.Equal("iou", tx.Namespaces()[0], "expected the [iou] namespace, got [%s]", tx.Namespaces()[0])

	// Commands are properly populated
	assert.Equal(1, tx.Commands().Count(), "expected only a single command, got [%s]", tx.Commands().Count())
	switch command := tx.Commands().At(0); command.Name {
	case "create":
		// If the create command is attached to the transaction then...

		// No inputs expected. The single output at index 0 should be an IOU state
		assert.Equal(0, tx.NumInputs(), "invalid number of inputs, expected 0, was [%d]", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was [%d]", tx.NumInputs())
		iouState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, iouState))

		assert.True(iouState.Amount >= 5, "invalid amount, expected at least 5, was [%d]", iouState.Amount)
		assert.Equal(2, iouState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", iouState.Owners().Count())
		assert.False(iouState.Owners()[0].Equal(iouState.Owners()[1]), "owner identities must be different")
		assert.True(iouState.Owners().Match(command.Ids), "invalid state, it does not contain command's identities")
		assert.NoError(tx.HasBeenEndorsedBy(iouState.Owners()...), "signatures are missing")
	case "update":
		// If the update command is attached to the transaction then...

		// The single input and output should be an IOU state
		assert.Equal(1, tx.NumInputs(), "invalid number of inputs, expected 1, was [%d]", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs,  expected 1, was [%d]", tx.NumInputs())

		inState := &states.IOU{}
		assert.NoError(tx.GetInputAt(0, inState))
		outState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, outState))

		assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, [%s] != [%s]", inState.LinearID, outState.LinearID)
		assert.True(outState.Amount < inState.Amount, "invalid amount, [%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
		assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, input and output should have the same owners")
		assert.NoError(tx.HasBeenEndorsedBy(outState.Owners()...), "signatures are missing")
	default:
		return nil, errors.Errorf("invalid command, expected [create] or [update], was [%s]", command)
	}

	// The approver is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the approver waits that the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx))
}

```

### Update the IOU State

Once the IOU state has been created, the parties can agree on the changes to the state
to reflect the evolution of the amount the borrower owes the lender.

Indeed, this is the view the borrower executes to update the IOU state's amount field.

```go

// Update contains the input to update an IOU state
type Update struct {
	// LinearID is the unique identifier of the IOU state
	LinearID string
	// Amount is the new amount. It should smaller than the current amount
	Amount uint
	// Approver is the identity of the approver's FSC node
	Approver view.Identity
}

type UpdateIOUView struct {
	Update
}

func (u UpdateIOUView) Call(context view.Context) (interface{}, error) {
    // use default identities if not specified
    if u.Approver.IsNone() {
        u.Approver = view2.GetIdentityProvider(context).Identity("approver")
    }

    // The borrower starts by creating a new transaction to update the IOU state
	tx, err := state.NewTransaction(context)
	assert.NoError(err)

	// Sets the namespace where the state is stored
	tx.SetNamespace("iou")

	// To update the state, the borrower, first add a dependency to the IOU state of interest.
	iouState := &states.IOU{}
	assert.NoError(tx.AddInputByLinearID(u.LinearID, iouState))
	// The borrower sets the command to the operation to be performed
	assert.NoError(tx.AddCommand("update", iouState.Owners()...))

	// Then, the borrower updates the amount,
	iouState.Amount = u.Amount

	// and add the modified IOU state as output of the transaction.
	err = tx.AddOutput(iouState)
	assert.NoError(err)

	// The borrower is ready to collect all the required signatures.
	// Namely from the borrower itself, the lender, and the approver. In this order.
	// All signatures are required.
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, iouState.Owners()[0], iouState.Owners()[1], u.Approver))
	assert.NoError(err)

	// At this point the borrower can send the transaction to the ordering service and wait for finality.
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err)

	// Return the state ID
	return iouState.LinearID, nil
}

```

On the other hand, the lender responds to request of endorsement from the borrower running the following view:

```go

type UpdateIOUResponderView struct{}

func (i *UpdateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the lender. Therefore, the lender waits to receive the transaction.
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// The lender can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	assert.Equal(1, len(tx.Namespaces()), "expected only one namespace")
	assert.Equal("iou", tx.Namespaces()[0], "expected the [iou] namespace, got [%s]", tx.Namespaces()[0])

	switch command := tx.Commands().At(0); command.Name {
	case "update":
		// If the update command is attached to the transaction then...

		// One input and one output containing IOU states are expected
		assert.Equal(1, tx.NumInputs(), "invalid number of inputs, expected 1, was %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was %d", tx.NumInputs())
		inState := &states.IOU{}
		assert.NoError(tx.GetInputAt(0, inState))
		outState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, outState))

		// Additional checks
		// Same IDs
		assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, [%s] != [%s]", inState.LinearID, outState.LinearID)
		// Valid Amount
		assert.False(outState.Amount >= inState.Amount, "invalid amount, [%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
		// Same owners
		assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, input and output should have the same owners")
		assert.Equal(2, inState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", inState.Owners().Count())
		// Is the lender one of the owners?
		lenderFound := fabric.GetLocalMembership(context).IsMe(inState.Owners()[0]) != fabric.GetLocalMembership(context).IsMe(inState.Owners()[1])
		assert.True(lenderFound, "lender identity not found")
		// Did the borrower sign?
		assert.NoError(tx.HasBeenEndorsedBy(inState.Owners().Filter(
			func(identity view.Identity) bool {
				return !fabric.GetLocalMembership(context).IsMe(identity)
			})...), "the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, expected [create], was [%s]", command.Name)
	}

	// The lender is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the lender waits that the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx))
}

```

We have seen already what the approver does.

## Testing

To run the `IOU` sample, one needs first to deploy the `Fabric Smart Client nodes` and the `Fabric network`.
Once these networks are deployed, one can invoke views on the smart client nodes to test the `IOU` sample.

So, first step is to describe the topology of the networks we need. 

### Describe the topology of the networks

To test the above views, we have to first clarify the topology of the networks we need.
Namely, Fabric and FSC networks.

For Fabric, we can define a topology with the following characteristics:
1. Three organization: Org1, Org2, and Org3;
2. Single channel;
2. A namespace `iou` whose changes can be endorsed by endorsers from Org1.

For the FSC network, we have a topology with:
1. 3 FCS nodes. One for the approver, one for the borrower, and one for the lender.
2. The approver's FSC node has an additional Fabric identity belonging to Org1.
   Therefore, the approver is an endorser of the Fabric namespace we defined above.

We can describe the network topology programmatically as follows:

```go
func Topology() []api.Topology {
    // Define a Fabric topology with:
    // 1. Three organization: Org1, Org2, and Org3
    // 2. A namespace whose changes can be endorsed by Org1.
    fabricTopology := fabric.NewDefaultTopology()
    fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
    fabricTopology.SetNamespaceApproverOrgs("Org1")
    fabricTopology.AddNamespaceWithUnanimity("iou", "Org1")
    fabricTopology.EnableGRPCLogging()
    fabricTopology.EnableLogPeersToFile()
    fabricTopology.SetLogging("info", "")
    
    // Define an FSC topology with 3 FCS nodes.
    // One for the approver, one for the borrower, and one for the lender.
    fscTopology := fsc.NewTopology()
    fscTopology.SetLogging("debug", "")
    fscTopology.EnableLogToFile()
    
    // Add the approver FSC node.
    approver := fscTopology.AddNodeByName("approver")
    // This option equips the approver's FSC node with an identity belonging to Org1.
    // Therefore, the approver is an endorser of the Fabric namespace we defined above.
    approver.AddOptions(
    fabric.WithOrganization("Org1"),
    fabric.WithX509Identity("alice"),
    )
    approver.RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{})
    approver.RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})
    
    // Add the borrower's FSC node
    borrower := fscTopology.AddNodeByName("borrower")
    borrower.AddOptions(fabric.WithOrganization("Org2"))
    borrower.RegisterViewFactory("create", &views.CreateIOUViewFactory{})
    borrower.RegisterViewFactory("update", &views.UpdateIOUViewFactory{})
    borrower.RegisterViewFactory("query", &views.QueryViewFactory{})
    
    // Add the lender's FSC node
    lender := fscTopology.AddNodeByName("lender")
    lender.AddOptions(
    fabric.WithOrganization("Org3"),
    fabric.WithX509Identity("bob"),
    )
    lender.RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{})
    lender.RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{})
    lender.RegisterViewFactory("query", &views.QueryViewFactory{})
    
    return []api.Topology{fabricTopology, fscTopology}
}
```

### Boostrap the networks

To help us bootstrap the networks and then invoke the business views, the `iou` command line tool is provided.
To build it, we need to run the following command:

```shell
go build -o iou
```

If the compilation is successful, we can run the `iou` command line tool as follows:

``` 
./iou network start --path ./testdata
```

The above command will start the Fabric network and the FSC network, and store all configuration files
under the `./testdata` directory.

If everything is successful, you will see something like the following:

```shell
2022-02-08 11:53:03.560 UTC [fsc.integration] Start -> INFO 027  _____   _   _   ____
2022-02-08 11:53:03.561 UTC [fsc.integration] Start -> INFO 028 | ____| | \ | | |  _ \
2022-02-08 11:53:03.561 UTC [fsc.integration] Start -> INFO 029 |  _|   |  \| | | | | |
2022-02-08 11:53:03.561 UTC [fsc.integration] Start -> INFO 02a | |___  | |\  | | |_| |
2022-02-08 11:53:03.561 UTC [fsc.integration] Start -> INFO 02b |_____| |_| \_| |____/
2022-02-08 11:53:03.561 UTC [fsc.integration] Serve -> INFO 02c All GOOD, networks up and running
```

To shut down the networks, just press CTRL-c.

If you want to restart the networks after the shutdown, you can just re-run the above command.
If you don't delete the `./testdata` directory, the network will be started from the previous state.

Before restarting the networks, one can modify the business views to add new functionalities, to fix bugs, and so on.
Upon restarting the networks, the new business views will be available.
Later on, we will see an example of this.

### Invoke the business views

If you reached this point, you can now invoke the business views on the FSC nodes.

To create an IOU, you can run the following command in a new terminal window:

```shell
./iou view -c ./testdata/fsc/nodes/borrower/client-config.yaml -f create -i "{\"Amount\":10}"
```

The above command invoke the `create` view on the borrower's FSC node. The `-c` option specifies the client configuration file.
The `-f` option specifies the view name. The `-i` option specifies the input data.
In the specific case, we are creating an IOU with amount 10. The lender and the approver are the default ones.
If everything is successful, you will see something like the following:

```shell
"bd90b6c8-0a54-4719-8caa-00759bad7d69"
```
The above is the IOU ID that we will use to update the IOU or query it.

Indeed, once the IOU is created, you can query the IOUs by running the following command:

```shell
./iou view -c ./testdata/fsc/nodes/borrower/client-config.yaml -f query -i "{\"LinearID\":\"bd90b6c8-0a54-4719-8caa-00759bad7d69\"}"
```

The above command will query the IOU with the linear ID `bd90b6c8-0a54-4719-8caa-00759bad7d69` on the borrower's FSC node.
If everything is successful, you will the current amount contained in the IOU state: 10.

If you want to query the IOU start on the lender node, you can run the following command:

```shell 
./iou view -c ./testdata/fsc/nodes/lender/client-config.yaml -f query -i "{\"LinearID\":\"bd90b6c8-0a54-4719-8caa-00759bad7d69\"}"
```

To update the IOU, you can run the following command:

```shell
./iou view -c ./testdata/fsc/nodes/borrower/client-config.yaml -f update -i "{\"LinearID\":\"bd90b6c8-0a54-4719-8caa-00759bad7d69\",\"Amount\":8}"
```

The above command will update the IOU with the linear ID `bd90b6c8-0a54-4719-8caa-00759bad7d69`. The new amount will be 8.

### Modify the business views and restart the networks

Suppose you want to change the behaviour of a business view and see it in actions, one can do the following:
1. Stop the networks;
2. Update the business views;
3. Restart the networks;

Let's see how to do this with a concrete example. When the borrower does not owe the lender anything anymore,
the borrower updates the IOU state to 0. The state still exists though. What we can do instead is to delete the state.
We can do that by replacing in the business view `UpdateIOUView`, the line
```go
    err = tx.AddOutput(iouState)
```
with
```go
	if iouState.Amount == 0 {
		err = tx.Delete(iouState)
	} else {
		err = tx.AddOutput(iouState)
	}
```
Now, we need to update the business view of the lender and the borrower to take in account the new behaviour.
For the lender, we modify `UpdateIOUResponderView` to check for the deleted state using the following code:

```go
    output := tx.Outputs().At(0)
    if !output.IsDelete() {
        outState := &states.IOU{}
        assert.NoError(tx.GetOutputAt(0, outState))

        // Additional checks
        // Same IDs
        assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, [%s] != [%s]", inState.LinearID, outState.LinearID)
        // Valid Amount
        assert.False(outState.Amount >= inState.Amount, "invalid amount, [%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
        // Same owners
        assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, input and output should have the same owners")
        assert.Equal(2, inState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", inState.Owners().Count())
    }
```

For the approver, we update the validation code for the `update` command in `ApproverView`:

```go
		output := tx.Outputs().At(0)
		if !output.IsDelete() {
			outState := &states.IOU{}
			assert.NoError(tx.GetOutputAt(0, outState))
			assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, [%s] != [%s]", inState.LinearID, outState.LinearID)
			assert.True(outState.Amount < inState.Amount, "invalid amount, [%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
			assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, input and output should have the same owners")
		}

		assert.NoError(tx.HasBeenEndorsedBy(inState.Owners()...), "signatures are missing")
```

Now, we can just restart the networks, the Fabric Smart Client nodes will be rebuilt and the new behaviour available.
You can test by yourself.





