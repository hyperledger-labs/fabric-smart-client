# Fabric Platform

The Fabric platform extends the FSC view runtime with services and APIs for communicating with Hyperledger Fabric networks.

## Start Here

- [Fabric SDK architecture](fabric-sdk.md)
- [Fabric platform driver architecture](drivers.md)
- [Fabric platform configuration](configuration.md)

## Use Fabric from a view

A node talks to Fabric once it installs the Fabric [SDK](../../../platform/fabric/sdk/dig/sdk.go) in place of the view SDK. The Fabric SDK builds on the view SDK, so the views and responders you register work as in the [Programming Model Guide](../view/programming-model.md). The SDK does nothing unless `fabric.enabled` is `true` in `core.yaml` (see [configuration](configuration.md)).

```go
import (
	"log"

	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(fabric.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		if err := registry.RegisterFactory("create", &CreateIOUViewFactory{}); err != nil {
			return err
		}
		// on the lender's node; the approver's node registers &ApproverView{} instead
		return registry.RegisterResponder(&LenderView{}, &CreateIOUView{})
	})
}
```

The views below create an IOU (I owe you) state, trimmed from the IOU example. The borrower assembles the transaction with the state-based API of [`platform/fabric/services/state`](../../../platform/fabric/services/state), which wraps the endorser transaction of [`platform/fabric/services/endorser`](../../../platform/fabric/services/endorser): it picks a namespace, adds a command naming the parties, and adds the new state as an output. `NewCollectEndorsementsView` sends the transaction to each party in turn and collects its signature; [Collecting endorsements](fabric-sdk.md#collecting-endorsements) describes the errors it returns when a party fails. The approver is an FSC node whose Fabric identity satisfies the endorsement policy of the `iou` namespace, so its signature is the endorsement Fabric checks. `NewOrderingAndFinalityWithTimeoutView` submits the transaction to the ordering service and waits until it is final; it returns an error if Fabric marks the transaction invalid or the timeout passes.

```go
import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IOU is the state the transaction stores in the "iou" namespace.
type IOU struct {
	Amount   uint
	LinearID string
	Parties  []view.Identity
}

// SetLinearID lets AddOutput assign the state's key.
func (i *IOU) SetLinearID(id string) string {
	if i.LinearID == "" {
		i.LinearID = id
	}
	return i.LinearID
}

// CreateIOUView runs on the borrower's node.
type CreateIOUView struct {
	Amount   uint
	Lender   view.Identity // the lender's FSC node
	Approver view.Identity // an FSC node with an endorsing identity for "iou"
}

func (c *CreateIOUView) Call(ctx view.Context) (any, error) {
	// Agree with the lender on the identities that own the new state.
	borrower, lender, err := state.ExchangeRecipientIdentities(ctx, c.Lender)
	if err != nil {
		return nil, err
	}

	// Assemble the transaction: namespace, command and output state.
	tx, err := state.NewTransaction(ctx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace("iou")
	if err := tx.AddCommand("create", borrower, lender); err != nil {
		return nil, err
	}
	iou := &IOU{Amount: c.Amount, Parties: []view.Identity{borrower, lender}}
	if err := tx.AddOutput(iou); err != nil {
		return nil, err
	}

	// Collect the signatures of the borrower, the lender and the approver.
	if _, err := ctx.RunView(state.NewCollectEndorsementsView(tx, borrower, lender, c.Approver)); err != nil {
		return nil, err
	}

	// Submit to the ordering service and wait until the transaction is committed.
	if _, err := ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return nil, err
	}
	return iou.LinearID, nil
}

type CreateIOUViewFactory struct{}

func (*CreateIOUViewFactory) NewView(in []byte) (view.View, error) {
	v := &CreateIOUView{}
	return v, json.Unmarshal(in, v)
}

// LenderView runs on the lender's node when CreateIOUView contacts it.
type LenderView struct{}

func (*LenderView) Call(ctx view.Context) (any, error) {
	if _, _, err := state.RespondExchangeRecipientIdentities(ctx); err != nil {
		return nil, err
	}

	// Receive the transaction NewCollectEndorsementsView sends and inspect it.
	tx, err := state.ReceiveTransaction(ctx)
	if err != nil {
		return nil, err
	}
	iou := &IOU{}
	if err := tx.GetOutputAt(0, iou); err != nil {
		return nil, err
	}
	if iou.Amount < 5 {
		return nil, errors.Errorf("amount [%d] is below 5", iou.Amount)
	}

	// Endorse it, then wait for finality.
	if _, err := ctx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, err
	}
	return ctx.RunView(state.NewFinalityWithTimeoutView(tx, time.Minute))
}
```

The approver's responder has the same shape as `LenderView` without the identity exchange: it receives the transaction, checks it against its own rules, endorses it and waits for finality. To reach a network or channel other than the default one, use `fabric.GetFabricNetworkService` and `fabric.GetChannel` from [`platform/fabric`](../../../platform/fabric/fns.go); `fabric.GetDefaultChannel` returns the default ones.

The complete example is [`integration/fabric/common/iou`](../../../integration/fabric/common/iou): the borrower, lender and approver views, including an update of an existing state, are in its `views` directory, and `topology.go` shows which node registers which view.

## What This Platform Adds

- Fabric-aware APIs layered on top of the View platform
- Transaction endorsement and submission flows
- Identity providers and finality handlers
- Vault and state-oriented services used by Fabric applications

## Related Documentation

- [View platform](../view/README.md)
- [Shared node configuration](../../configuration.md)
