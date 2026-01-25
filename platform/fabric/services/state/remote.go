/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// FetchState retrieves a state from the QueryService (sidecar).
// This is used to fetch states that were created by other FSC nodes
// and are not available in the local vault.
//
// Parameters:
//   - ctx: View context providing access to services
//   - network: Network name
//   - channel: Channel name
//   - namespace: The namespace (e.g., "tokenx")
//   - key: The state key (linear ID)
//
// Returns:
//   - *driver.VaultValue containing the raw state bytes and version
//   - error if fetching fails or state is not found
func FetchState(ctx view.Context, network, channel, namespace, key string) (*driver.VaultValue, error) {
	qs, err := queryservice.GetQueryService(ctx, network, channel)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get QueryService")
	}

	val, err := qs.GetState(driver.Namespace(namespace), driver.PKey(key))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch state [%s:%s]", namespace, key)
	}

	if val == nil {
		return nil, errors.Errorf("state not found [%s:%s]", namespace, key)
	}

	return val, nil
}

// FetchStateFromDefaultChannel retrieves a state from the QueryService using the default channel.
//
// Parameters:
//   - ctx: View context providing access to services
//   - namespace: The namespace (e.g., "tokenx")
//   - key: The state key (linear ID)
//
// Returns:
//   - *driver.VaultValue containing the raw state bytes and version
//   - error if fetching fails or state is not found
func FetchStateFromDefaultChannel(ctx view.Context, namespace, key string) (*driver.VaultValue, error) {
	network, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get default channel")
	}

	return FetchState(ctx, network.Name(), ch.Name(), namespace, key)
}

// AddRemoteInput fetches a state from the QueryService and adds it as a transaction input.
// This bypasses the local vault limitation when the state was created by a different node.
//
// The function:
// 1. Fetches the state and version from the sidecar QueryService
// 2. Calls AddInputByLinearID with the pre-fetched raw value and version
// 3. Records the read dependency for MVCC validation
//
// Parameters:
//   - ctx: View context
//   - tx: The state transaction to add the input to
//   - namespace: The namespace (e.g., "tokenx")
//   - linearID: The linear ID of the state to fetch
//   - state: Pointer to the struct to unmarshal the state into
//
// Returns:
//   - error if fetching or adding the input fails
func AddRemoteInput(ctx view.Context, tx *Transaction, namespace, linearID string, state interface{}) error {
	network, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get default channel")
	}

	return AddRemoteInputWithChannel(ctx, tx, network.Name(), ch.Name(), namespace, linearID, state)
}

// AddRemoteInputWithChannel fetches a state from the QueryService and adds it as a transaction input,
// allowing explicit specification of network and channel.
//
// Parameters:
//   - ctx: View context
//   - tx: The state transaction to add the input to
//   - network: Network name
//   - channel: Channel name
//   - namespace: The namespace (e.g., "tokenx")
//   - linearID: The linear ID of the state to fetch
//   - state: Pointer to the struct to unmarshal the state into
//
// Returns:
//   - error if fetching or adding the input fails
func AddRemoteInputWithChannel(ctx view.Context, tx *Transaction, network, channel, namespace, linearID string, state interface{}) error {
	val, err := FetchState(ctx, network, channel, namespace, linearID)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch remote state [%s:%s]", namespace, linearID)
	}

	logger.Debugf("[AddRemoteInput] Retrieved state [%s:%s]: %d bytes, version: %v",
		namespace, linearID, len(val.Raw), val.Version)

	if err := tx.AddInputByLinearID(linearID, state, WithRawValue(val.Raw, val.Version)); err != nil {
		return errors.Wrapf(err, "failed to add input by linear ID [%s]", linearID)
	}

	logger.Debugf("[AddRemoteInput] Successfully added remote input [%s:%s]", namespace, linearID)
	return nil
}

// BatchFetchStates retrieves multiple states from the QueryService in a single RPC call.
// This is more efficient than making multiple individual FetchState calls.
//
// Parameters:
//   - ctx: View context
//   - network: Network name
//   - channel: Channel name
//   - namespace: The namespace
//   - keys: Slice of state keys to fetch
//
// Returns:
//   - map[string]*driver.VaultValue mapping keys to their values
//   - error if fetching fails
func BatchFetchStates(ctx view.Context, network, channel, namespace string, keys []string) (map[string]*driver.VaultValue, error) {
	if len(keys) == 0 {
		return make(map[string]*driver.VaultValue), nil
	}

	qs, err := queryservice.GetQueryService(ctx, network, channel)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get QueryService")
	}

	keySlice := make([]driver.PKey, len(keys))
	for i, k := range keys {
		keySlice[i] = driver.PKey(k)
	}

	m := map[driver.Namespace][]driver.PKey{
		driver.Namespace(namespace): keySlice,
	}

	results, err := qs.GetStates(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to batch fetch states")
	}

	output := make(map[string]*driver.VaultValue)
	if nsResults, ok := results[driver.Namespace(namespace)]; ok {
		for k, v := range nsResults {
			vCopy := v
			output[string(k)] = &vCopy
		}
	}

	return output, nil
}

// BatchFetchStatesFromDefaultChannel retrieves multiple states using the default channel.
//
// Parameters:
//   - ctx: View context
//   - namespace: The namespace
//   - keys: Slice of state keys to fetch
//
// Returns:
//   - map[string]*driver.VaultValue mapping keys to their values
//   - error if fetching fails
func BatchFetchStatesFromDefaultChannel(ctx view.Context, namespace string, keys []string) (map[string]*driver.VaultValue, error) {
	network, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get default channel")
	}

	return BatchFetchStates(ctx, network.Name(), ch.Name(), namespace, keys)
}

// RemoteInput represents a state to be added as a remote input.
type RemoteInput struct {
	LinearID string
	State    interface{}
}

// AddRemoteInputs fetches multiple states from the QueryService and adds them as transaction inputs.
// This uses batch fetching for efficiency.
//
// Parameters:
//   - ctx: View context
//   - tx: The state transaction to add inputs to
//   - namespace: The namespace
//   - inputs: Slice of RemoteInput containing linear IDs and state pointers
//
// Returns:
//   - error if fetching or adding any input fails
func AddRemoteInputs(ctx view.Context, tx *Transaction, namespace string, inputs []RemoteInput) error {
	network, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get default channel")
	}

	return AddRemoteInputsWithChannel(ctx, tx, network.Name(), ch.Name(), namespace, inputs)
}

// AddRemoteInputsWithChannel fetches multiple states and adds them as transaction inputs,
// allowing explicit specification of network and channel.
//
// Parameters:
//   - ctx: View context
//   - tx: The state transaction to add inputs to
//   - network: Network name
//   - channel: Channel name
//   - namespace: The namespace
//   - inputs: Slice of RemoteInput containing linear IDs and state pointers
//
// Returns:
//   - error if fetching or adding any input fails
func AddRemoteInputsWithChannel(ctx view.Context, tx *Transaction, network, channel, namespace string, inputs []RemoteInput) error {
	if len(inputs) == 0 {
		return nil
	}

	// Collect all keys for batch fetch
	keys := make([]string, len(inputs))
	for i, input := range inputs {
		keys[i] = input.LinearID
	}

	// Batch fetch all states
	vals, err := BatchFetchStates(ctx, network, channel, namespace, keys)
	if err != nil {
		return errors.Wrap(err, "failed to batch fetch states")
	}

	// Add each input
	for _, input := range inputs {
		val, ok := vals[input.LinearID]
		if !ok || val == nil {
			return errors.Errorf("state not found [%s:%s]", namespace, input.LinearID)
		}

		logger.Debugf("[AddRemoteInputs] Adding input [%s:%s]: %d bytes, version: %v",
			namespace, input.LinearID, len(val.Raw), val.Version)

		if err := tx.AddInputByLinearID(input.LinearID, input.State, WithRawValue(val.Raw, val.Version)); err != nil {
			return errors.Wrapf(err, "failed to add input by linear ID [%s]", input.LinearID)
		}
	}

	logger.Debugf("[AddRemoteInputs] Successfully added %d remote inputs", len(inputs))
	return nil
}
