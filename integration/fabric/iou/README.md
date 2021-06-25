# I O(we) (yo)U

In this section, we will cover a classic use-case, the `I O(we) (yo)U`.
Let us consider two parties: a `lender` and a `borrower`. 
The borrower owes the lender a certain amount of money. 
Both parties want to track the evolution of this amount.
Moreover, the parties want an `approver` to validate their operations. 
We can think about the approver as a mediator between the parties.
(Looking ahead, the approver plays the role of the endorser of the namespace
in a Fabric channel that contains the IOU state.)

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
Let us assume that the borrower is the initiation of the interactive protocol to create this state.
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
	return context.RunView(state.NewOrderingAndFinalityView(tx))
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

Normally, to run the `IOU` sample, one would have to deploy the Fabric Smart Client nodes, the Fabric networks,
invoke the view, and so on, by using a bunch of scripts.
This is not the most convenient way to test programmatically an application.

FSC provides an `Integration Test Infrastructure` that allow the developer to:
- Describe the topology of the networks (FSC and Fabric networks, in this case);
- Boostrap these networks;
- Initiate interactive protocols to complete given business tasks.

To run the test, it is just enough to run `go test` from the folder containing the test.

Let us go step by step.

### Describe the topology of the networks

To test the above views, we have to first clarify the topology of the networks we need. 
Namely, Fabric and FSC networks.

For Fabric, we can define a topology with the following characteristics:
1. Three organization: Org1, Org2, and Org3
2. A namespace whose changes can be endorsed by Org1.

For the FSC network, we have a topology with: 
1. 3 FCS nodes. One for the approver, one for the borrower, and one for the lender.
2. The approver's FSC node is equipped with an additional Fabric identity belonging to Org1.
Therefore, the approver is an endorser of the Fabric namespace we defined above.
   
We can describe the network topology programmatically as follows:

```go

func Topology() []nwo.Topology {
	// Define a Fabric topology with:
	// 1. Three organization: Org1, Org2, and Org3
	// 2. A namespace whose changes can be endorsed by Org1.
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("iou", "Org1")

	// Define an FSC topology with 3 FCS nodes.
	// One for the approver, one for the borrower, and one for the lender.
	fscTopology := fsc.NewTopology()

	// Add the approver FSC node.
	approver := fscTopology.AddNodeByName("approver")
	// This option equips the approver's FSC node with an identity belonging to Org1.
	// Therefore, the approver is an endorser of the Fabric namespace we defined above.
	approver.AddOptions(fabric.WithOrganization("Org1"))
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
	lender.AddOptions(fabric.WithOrganization("Org3"))
	lender.RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{})
	lender.RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{})
	lender.RegisterViewFactory("query", &views.QueryViewFactory{})

	return []nwo.Topology{fabricTopology, fscTopology}
}

```

### Boostrap these networks and Start a Business Process

This is very similar to what we have seen already for the [`Ping Pong` sample](./../../fsc/pingpong/README.md#boostrap-these-networks)
