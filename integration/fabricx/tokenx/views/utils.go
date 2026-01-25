/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// FinalityListener waits for a transaction to reach a specific validation status
type FinalityListener struct {
	ExpectedTxID string
	ExpectedVC   fdriver.ValidationCode
	WaitGroup    *sync.WaitGroup
}

// NewFinalityListener creates a new FinalityListener
func NewFinalityListener(expectedTxID string, expectedVC fdriver.ValidationCode, waitGroup *sync.WaitGroup) *FinalityListener {
	return &FinalityListener{
		ExpectedTxID: expectedTxID,
		ExpectedVC:   expectedVC,
		WaitGroup:    waitGroup,
	}
}

// OnStatus is called when a transaction status update is received
func (f *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, message string) {
	if txID == f.ExpectedTxID && vc == f.ExpectedVC {
		time.Sleep(5 * time.Second) // Short delay to ensure state is committed
		f.WaitGroup.Done()
	}
}

// FinalityTimeout is the default timeout for waiting on transaction finality
const FinalityTimeout = 60 * time.Second

// AddRemoteInput fetches a state from the sidecar QueryService and adds it as an input
// to the transaction. This bypasses the local vault limitation when the state was
// created by a different node.
//
// Deprecated: Use state.AddRemoteInput from platform/fabric/services/state package instead.
// This function is kept for backward compatibility and delegates to the platform helper.
//
// Parameters:
//   - ctx: View context
//   - tx: The state transaction to add the input to
//   - namespace: The namespace (e.g., "tokenx")
//   - linearID: The linear ID of the state to fetch
//   - stateObj: Pointer to the struct to unmarshal the state into
//
// Returns:
//   - error if fetching or adding the input fails
func AddRemoteInput(ctx view.Context, tx *state.Transaction, namespace, linearID string, stateObj interface{}) error {
	// Delegate to platform helper
	return state.AddRemoteInput(ctx, tx, namespace, linearID, stateObj)
}

// ValidateNamespaceRWSetKeys checks for empty keys in reads and writes for the given namespace.
func ValidateNamespaceRWSetKeys(tx *state.Transaction, namespace string) error {
	rwSet, err := tx.Namespace.RWSet()
	if err != nil {
		return errors.Wrap(err, "failed getting RWSet")
	}

	for i := 0; i < rwSet.NumReads(namespace); i++ {
		k, err := rwSet.GetReadKeyAt(namespace, i)
		if err != nil {
			return errors.Wrapf(err, "failed getting read key at %d", i)
		}
		if len(k) == 0 {
			return errors.Errorf("empty read key at index %d for namespace %s", i, namespace)
		}
	}

	for i := 0; i < rwSet.NumWrites(namespace); i++ {
		k, _, err := rwSet.GetWriteAt(namespace, i)
		if err != nil {
			return errors.Wrapf(err, "failed getting write key at %d", i)
		}
		if len(k) == 0 {
			return errors.Errorf("empty write key at index %d for namespace %s", i, namespace)
		}
	}

	return nil
}
